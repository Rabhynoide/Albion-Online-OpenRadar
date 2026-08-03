// Throwaway analysis script: dumps Market/Auction-related Photon events/requests/responses
// from a pcap, so we can reverse-engineer which parameters carry item/price/quality/city on
// marketplace browsing (issue #23, Part B). Not part of the build; delete after use.
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
var marketEventCodes = map[int16]string{
	58:  "MarketPlaceBuildingInfo",
	183: "MarketPlaceNotification",
	466: "EstimatedMarketValueUpdate",
}

// Operation codes (real code, params[253] for request/response) from OperationCodes.js
var marketOpCodes = map[int16]string{
	79:  "AuctionCreateOffer",
	80:  "AuctionCreateRequest",
	81:  "AuctionGetOffers",
	82:  "AuctionGetRequests",
	83:  "AuctionBuyOffer",
	84:  "AuctionAbortAuction",
	85:  "AuctionModifyAuction",
	86:  "AuctionAbortOffer",
	87:  "AuctionAbortRequest",
	88:  "AuctionSellRequest",
	89:  "AuctionGetFinishedAuctions",
	90:  "AuctionGetFinishedAuctionsCount",
	91:  "AuctionFetchAuction",
	92:  "AuctionGetMyOpenOffers",
	93:  "AuctionGetMyOpenRequests",
	94:  "AuctionGetMyOpenAuctions",
	95:  "AuctionGetItemAverageStats",
	96:  "AuctionGetItemAverageValue",
	97:  "AuctionGetLowestOfferPrices",
	315: "AuctionSellSpecificItemRequest",
	413: "RequestEstimatedMarketValue",
	454: "AuctionGetLoadoutOffers",
	455: "AuctionBuyLoadoutOffer",
	484: "QuickSellAuctionQueryAction",
	485: "QuickSellAuctionSellAction",
	489: "AuctionFetchFinishedAuctions",
	490: "AbortAuctionFetchFinishedAuctions",
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: tmp-dump-market <capture.anon.pcap>")
		os.Exit(2)
	}
	path := os.Args[1]

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")

	parser := photon.NewPhotonParser(
		func(e *photon.EventData) {
			photon.PostProcessEvent(e)
			code, _ := e.Parameters[252].(int16)
			if name, ok := marketEventCodes[code]; ok {
				_ = enc.Encode(map[string]interface{}{"kind": "event", "name": name, "code": code, "params": e.Parameters})
			}
		},
		func(r *photon.OperationRequest) {
			photon.PostProcessRequest(r)
			code, _ := r.Parameters[253].(int16)
			if name, ok := marketOpCodes[code]; ok {
				_ = enc.Encode(map[string]interface{}{"kind": "request", "name": name, "code": code, "params": r.Parameters})
			}
		},
		func(r *photon.OperationResponse) {
			photon.PostProcessResponse(r)
			code, _ := r.Parameters[253].(int16)
			if name, ok := marketOpCodes[code]; ok {
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
