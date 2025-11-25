
/*
 * Copyright (c) 2024 Contributors to the Eclipse Foundation
 *
 *  All rights reserved. This program and the accompanying materials
 *  are made available under the terms of the Eclipse Public License v2.0
 *  and Eclipse Distribution License v1.0 which accompany this distribution.
 *
 * The Eclipse Public License is available at
 *    https://www.eclipse.org/legal/epl-2.0/
 *  and the Eclipse Distribution License is available at
 *    http://www.eclipse.org/org/documents/edl-v10.php.
 *
 *  SPDX-License-Identifier: EPL-2.0 OR BSD-3-Clause
 */

package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"log"
	"encoding/json"
	"regexp"
	"gopkg.in/yaml.v3"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
)

// Change this to something random if using a public test server
const clientID = "GoLights"
const mqttAddr = "192.168.0.103"

type LightMapping struct {
    SwitchID   string `yaml:"switch_id"`
    LightID    string `yaml:"light_id"`
    Brightness *int   `yaml:"brightness"`  // optional
}

type Config struct {
    Mappings []LightMapping `yaml:"mappings"`
}

var reSwitchCmd = regexp.MustCompile(`^zigbee2mqtt/([^/]+)/action$`)

func main() {

	// App will run until cancelled by user (e.g. ctrl-c)
	ctx, stop := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM)

	defer stop()

	// We will connect to the Eclipse test server
	// (note that you may see messages that other users publish)
	u, err := url.Parse("mqtt://" + mqttAddr + ":1883")
	if err != nil {
		panic(err)
	}

    // Channel for inbound messages
    incoming := make(chan paho.PublishReceived, 100)

	cliCfg := autopaho.ClientConfig{
		ServerUrls: []*url.URL{u},
		KeepAlive:  20, // Keepalive message should be sent every 20 seconds

		// CleanStartOnInitialConnection defaults to false.
		// Setting this to true will clear the session on the first connection.
		CleanStartOnInitialConnection: true,

		// SessionExpiryInterval - Seconds that a session will survive after disconnection.
		// 
		// It is important to set this because otherwise, any queued messages
		// will be lost if the connection drops and the server will not queue
		// messages while it is down.
		// 
		// The specific setting will depend upon your needs
		//   (60 = 1 minute, 3600 = 1 hour, 86400 = one day,
		//    0xFFFFFFFE = 136 years, 0xFFFFFFFF = don't expire)
		SessionExpiryInterval: 60,

		OnConnectionUp: func(cm *autopaho.ConnectionManager, connAck *paho.Connack) {

			fmt.Println("mqtt connection up")

			// Subscribing in the OnConnectionUp callback is recommended
			// (ensures the subscription is reestablished if the connection drops)
			if _, err := cm.Subscribe(context.Background(), &paho.Subscribe{
				Subscriptions: []paho.SubscribeOptions{
					{Topic: "zigbee2mqtt/+/action", QoS: 1},
					//{Topic: "#", QoS: 1},
				},
			}); err != nil {
				fmt.Printf("failed to subscribe (%s). This is likely to mean no messages will be received.", err)
			}

			fmt.Println("mqtt subscription made")
		},

		OnConnectError: func(err error) {
			fmt.Printf("error whilst attempting connection: %s\n", err)
		},

		// eclipse/paho.golang/paho provides base mqtt functionality,
		// the below config will be passed in for each connection
		ClientConfig: paho.ClientConfig {

			// If you are using QOS 1/2, then it's important to specify
			// a client id (which must be unique)
			ClientID: clientID,

			// OnPublishReceived is a slice of functions that will
			// be called when a message is received.
			// You can write the function(s) yourself or use the supplied Router
			OnPublishReceived: []func(paho.PublishReceived) (bool, error) {
				func(pr paho.PublishReceived) (bool, error) {

					incoming <- pr

					return true, nil
				},
			},

			OnClientError: func(err error) {
				fmt.Printf("client error: %s\n", err)
			},

			OnServerDisconnect: func(d *paho.Disconnect) {
				if d.Properties != nil {
					fmt.Printf("server requested disconnect: %s\n",
						d.Properties.ReasonString)
				} else {
					fmt.Printf("server requested disconnect; reason code: %d\n",
						d.ReasonCode)
				}
			},
		},
	}

	// Starts process; will reconnect until context cancelled
	c, err := autopaho.NewConnection(ctx, cliCfg)
	if err != nil {
		panic(err)
	}

	// Wait for the connection to come up
	if err = c.AwaitConnection(ctx); err != nil {
		panic(err)
	}
	
    log.Println("Service started.")

	var cfg Config
	data, _ := os.ReadFile("config.yaml")
	yaml.Unmarshal(data, &cfg)

	switchToLight := make(map[string]LightMapping)

	for _, m := range cfg.Mappings {
		switchToLight[m.SwitchID] = m
	}

	var lightState = map[string]bool{}
	
    // ---- Simple message loop ----
    for {
        select {
        case p := <-incoming:
            fmt.Printf("[mqtt] %s -> %s\n",
				p.Packet.Topic, string(p.Packet.Payload))

			matches := reSwitchCmd.FindStringSubmatch(p.Packet.Topic)

			if matches == nil {

				log.Println("unknown topic")
				continue
			}
			
			// extract from regex
			switchID := matches[1]

			l, ok := switchToLight[switchID]

			if ok {

				// e.g. "toggle", "on", "off"
				action := string(p.Packet.Payload)

				switch string(action) {

				case "single":
					log.Println("single")

					st, ok := lightState[l.LightID]

					if !ok {
						st = false;
					}

					var newState string

					if st {
						newState = "OFF"
						lightState[l.LightID] = false
					} else {
						newState = "ON"
						lightState[l.LightID] = true
					}

					cObj := map[string]any{
						"state":newState,
					}

					if newState == "ON" && l.Brightness != nil {
						cObj["brightness"] = *l.Brightness
					}

					var data []byte

					data, err = json.Marshal(cObj)

					if err != nil {
						if ctx.Err() == nil {
							panic(err) 
						}
					}

					if _, err := c.Publish(ctx, &paho.Publish{
						QoS:     1,
						Topic:   "hmd/light/MQTT-Lightwave-RF/" + l.LightID + "/command",
						Retain:  false,
						Payload: data,
					}); err != nil {
						if ctx.Err() == nil {
							// Publish will exit when context cancelled or if something went wrong
							panic(err) 
						}
					}

				default:
					log.Println("unknown action")
				}
			} else {

				log.Println("unknown switch:", switchID)
				continue
			}

        case <-ctx.Done():
            log.Println("Shutting down")
            return
        }
    }
}

