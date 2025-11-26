# go-ha-lainn
Go Hálainn!

"Go hálainn" is an Irish phrase that means _"beautiful"_ when used on its own, or as in "Tá sí go hálainn" (She is beautiful).

I hope that this simple exercise in lighting control will produce beautiful results.

## Rationale

I have been using _Home Assistant_ for many years and it's a great thing for linking things up, but there have been occasions where I've had to reach behind it and do some of my own scripting and I've noticed that over the years my scripted solutions haven't needed as much attention. I also find the process of implementing _Home Assistant_ automations _at scale_ to be quite cumbersome, and I have longed for a more lightweight "programmatic" approach. I've also had latency issues with home assistant on a buggy server, but even at the best of time I've a niggling feeling there is some additional latency being introduced ...

MQTT forms the basis of my approach for solving these problems. _Home Assistant_ has amazing MQTT support and this links to most of my control devices via Zigbee2MQTT. It is a very straightforward process to develop adaptors for devices that are not MQTT-native.

MQTT introduces a lightweight signalling fabric that makes it very straightforward to implement simple control plane logic. A simple script can become an active participant by connecting a lightweight client to the MQTT hub, and from there it is a matter of subscribing to the events of interest (e.g button press, temperature, presence detection), and sending control messages in response, or other automation activities.

## Instructions

Map switches to light identifiers in `config.yaml`

    mappings:
     - switch_id: SIDE_TUNNEL
       light_id: "Side-Tunnel"

     - switch_id: UPSTAIRS_MASTER_WINDOW
       light_id: "Master"
       brightness: 50

     - switch_id: UPSTAIRS_LANDING
       light_id: "Landing"
       brightness: 30

`switch_id` is the Zigbee device identifier
`light_id` is the LightwaveRF light identifier

Only Zigbee switches (Zigbee2MQTT) and LightwaveRF lights (exposed to MQTT using another script) are supported now, but it should be straightforward to extend to other types of devices.
