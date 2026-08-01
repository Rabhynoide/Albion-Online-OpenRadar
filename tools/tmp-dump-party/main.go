// Throwaway analysis script: dumps Party-related Photon events/requests/responses from a
// pcap, so we can reverse-engineer which parameters carry player name/id on
// join/leave/disband. Not part of the build; delete after use.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"

	"github.com/nospy/albion-openradar/internal/photon"
)

// Event codes (real code, params[252]) from web/scripts/utils/EventCodes.js
var partyEventCodes = map[int16]string{
	229: "PartyInvitation",
	230: "PartyJoinRequest",
	231: "PartyJoined",
	232: "PartyDisbanded",
	233: "PartyPlayerJoined",
	234: "PartyChangedOrder",
	235: "PartyPlayerLeft",
	236: "PartyLeaderChanged",
	237: "PartyLootSettingChangedPlayer",
	238: "PartySilverGained",
	239: "PartyPlayerUpdated",
	240: "PartyInvitationAnswer",
	241: "PartyJoinRequestAnswer",
	242: "PartyMarkedObjectsUpdated",
	243: "PartyOnClusterPartyJoined",
	244: "PartySetRoleFlag",
	245: "PartyInviteOrJoinPlayerEquipmentInfo",
	246: "PartyReadyCheckUpdate",
	509: "PartyPlayerLeaveScheduled",
}

// Operation codes (real code, params[253] for request/response) from OperationCodes.js
var partyOpCodes = map[int16]string{
	225: "PartyInvitePlayer",
	226: "PartyRequestJoin",
	227: "PartyAnswerInvitation",
	228: "PartyAnswerJoinRequest",
	229: "PartyLeave",
	230: "PartyKickPlayer",
	231: "PartyMakeLeader",
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: tmp-dump-party <capture.anon.pcap>")
		os.Exit(2)
	}
	path := os.Args[1]

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")

	parser := photon.NewPhotonParser(
		func(e *photon.EventData) {
			photon.PostProcessEvent(e)
			code, _ := e.Parameters[252].(int16)
			if name, ok := partyEventCodes[code]; ok {
				_ = enc.Encode(map[string]interface{}{"kind": "event", "name": name, "code": code, "params": e.Parameters})
			}
		},
		func(r *photon.OperationRequest) {
			photon.PostProcessRequest(r)
			code, _ := r.Parameters[253].(int16)
			if name, ok := partyOpCodes[code]; ok {
				_ = enc.Encode(map[string]interface{}{"kind": "request", "name": name, "code": code, "params": r.Parameters})
			}
		},
		func(r *photon.OperationResponse) {
			photon.PostProcessResponse(r)
			code, _ := r.Parameters[253].(int16)
			if name, ok := partyOpCodes[code]; ok {
				_ = enc.Encode(map[string]interface{}{"kind": "response", "name": name, "code": code, "params": r.Parameters})
			}
		},
	)

	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		os.Exit(1)
	}
	defer f.Close()

	r, err := pcapgo.NewReader(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pcap reader:", err)
		os.Exit(1)
	}

	for {
		data, _, err := r.ReadPacketData()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "read packet:", err)
			os.Exit(1)
		}
		pkt := gopacket.NewPacket(data, r.LinkType(), gopacket.Default)
		udp, _ := pkt.Layer(layers.LayerTypeUDP).(*layers.UDP)
		if udp == nil {
			continue
		}
		parser.ReceivePacket(udp.Payload)
	}
}
