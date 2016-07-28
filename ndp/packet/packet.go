//
//Copyright [2016] [SnapRoute Inc]
//
//Licensed under the Apache License, Version 2.0 (the "License");
//you may not use this file except in compliance with the License.
//You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
//	 Unless required by applicable law or agreed to in writing, software
//	 distributed under the License is distributed on an "AS IS" BASIS,
//	 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//	 See the License for the specific language governing permissions and
//	 limitations under the License.
//
// _______  __       __________   ___      _______.____    __    ____  __  .___________.  ______  __    __
// |   ____||  |     |   ____\  \ /  /     /       |\   \  /  \  /   / |  | |           | /      ||  |  |  |
// |  |__   |  |     |  |__   \  V  /     |   (----` \   \/    \/   /  |  | `---|  |----`|  ,----'|  |__|  |
// |   __|  |  |     |   __|   >   <       \   \      \            /   |  |     |  |     |  |     |   __   |
// |  |     |  `----.|  |____ /  .  \  .----)   |      \    /\    /    |  |     |  |     |  `----.|  |  |  |
// |__|     |_______||_______/__/ \__\ |_______/        \__/  \__/     |__|     |__|      \______||__|  |__|
//
package packet

import (
	"errors"
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"l3/ndp/config"
	"net"
)

func Init(pktCh chan config.PacketData) *Packet {
	pkt := &Packet{
		PktCh: pktCh,
	}
	pkt.LinkInfo = make(map[string]Link, 100)
	return pkt
}

func getEthLayer(pkt gopacket.Packet, eth *layers.Ethernet) error {
	ethLayer := pkt.Layer(layers.LayerTypeEthernet)
	if ethLayer == nil {
		return errors.New("Decoding ethernet layer failed")
	}
	*eth = *ethLayer.(*layers.Ethernet)
	return nil
}

/*
 *			ICMPv6 MESSAGE FORMAT
 *
 *    0                   1                   2                   3
 *    0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
 *   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
 *   |     Type      |     Code      |          Checksum             |
 *   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
 *   |                           Reserved                            |
 *   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
 *   |                                                               |
 *   +                                                               +
 *   |                                                               |
 *   +                       Target Address                          +
 *   |                                                               |
 *   +                                                               +
 *   |                                                               |
 *   +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
 *   |   Options ...
 *   +-+-+-+-+-+-+-+-+-+-+-+-
 *
 *  API: given a packet it will fill in ip header and icmpv6
 */
func getIpAndICMPv6Hdr(pkt gopacket.Packet, ipv6Hdr *layers.IPv6, icmpv6Hdr *layers.ICMPv6) error {
	ipLayer := pkt.Layer(layers.LayerTypeIPv6)
	if ipLayer == nil {
		return errors.New("Invalid IPv6 layer")
	}
	*ipv6Hdr = *ipLayer.(*layers.IPv6)
	ipPayload := ipLayer.LayerPayload()
	icmpv6Hdr.DecodeFromBytes(ipPayload, nil)
	return nil
}

func validateIPv6Hdr(hdr *layers.IPv6) error {
	if hdr.HopLimit != HOP_LIMIT {
		return errors.New(fmt.Sprintln("Invalid Hop Limit", hdr.HopLimit))
	}
	if hdr.Length < ICMPV6_MIN_LENGTH {
		return errors.New(fmt.Sprintln("Invalid ICMP length", hdr.Length))
	}
	return nil
}

func (p *Packet) decodeICMPv6Hdr(hdr *layers.ICMPv6, srcIP net.IP, dstIP net.IP) (*NDInfo, error) {
	ndInfo := &NDInfo{}
	var err error
	typeCode := hdr.TypeCode
	if typeCode.Code() != ICMPV6_CODE {
		return nil, errors.New(fmt.Sprintln("Invalid Code", typeCode.Code()))
	}
	switch typeCode.Type() {
	case layers.ICMPv6TypeNeighborSolicitation:
		ndInfo, err = p.HandleNSMsg(hdr, srcIP, dstIP)

	case layers.ICMPv6TypeNeighborAdvertisement:
		ndInfo, err = p.HandleNAMsg(hdr, srcIP, dstIP)

	case layers.ICMPv6TypeRouterSolicitation:
		return nil, errors.New("Router Solicitation is not yet supported")
	default:
		return nil, errors.New(fmt.Sprintln("Not Supported ICMPv6 Type:", typeCode.Type()))
	}
	if err != nil {
		return nil, err
	}
	return ndInfo, nil
}

func (p *Packet) populateNeighborInfo(nbrInfo *config.NeighborInfo, eth *layers.Ethernet, ipv6Hdr *layers.IPv6,
	icmpv6Hdr *layers.ICMPv6, ndInfo *NDInfo) {
	if eth == nil || ipv6Hdr == nil || icmpv6Hdr == nil {
		return
	}
	nbrInfo.MacAddr = (eth.SrcMAC).String()
	nbrInfo.IpAddr = (ipv6Hdr.SrcIP).String()
	nbrInfo.LinkLocalIp = ndInfo.TargetAddress.String()
	// Update Link information and Neigbor Cache with state
	link, _ := p.GetLink(ndInfo.TargetAddress.String())
	if entry, exists := link.NbrCache[ipv6Hdr.SrcIP.String()]; exists {
		nbrInfo.State = entry.State
	} else {
		nbrInfo.PktOperation = byte(PACKET_DROP)
	}
}

/* API: Get IPv6 & ICMPv6 Header
 *      Does Validation of IPv6
 *      Does Validation of ICMPv6
 * Validation Conditions are defined below, if anyone of them do not satisfy discard the packet:
 *  - The IP Hop Limit field has a value of 255, i.e., the packet
 *   could not possibly have been forwarded by a router. <- done
 *
 *  - ICMP Checksum is valid. <- done
 *
 *  - ICMP Code is 0. <- done
 *
 *  - ICMP length (derived from the IP length) is 24 or more octets. <- done
 *
 *  - Target Address is not a multicast address. <- done
 *
 *  - All included options have a length that is greater than zero. <- @TODO: need to add this later
 *
 *  - If the IP source address is the unspecified address, the IP
 *    destination address is a solicited-node multicast address. <- done
 *
 *  - If the IP source address is the unspecified address, there is no
 *    source link-layer address option in the message. <- @TODO: need to be done later
 */
func (p *Packet) ValidateAndParse(nbrInfo *config.NeighborInfo, pkt gopacket.Packet) error {
	// first decode all the layers
	icmpv6Hdr := &layers.ICMPv6{}
	ipv6Hdr := &layers.IPv6{}
	eth := &layers.Ethernet{}
	var err error

	// Get Ethernet Layer
	err = getEthLayer(pkt, eth)
	if err != nil {
		return err
	}

	// First get ipv6 and icmp6 information
	err = getIpAndICMPv6Hdr(pkt, ipv6Hdr, icmpv6Hdr)
	if err != nil {
		return err
	}

	// Validating ipv6 header
	err = validateIPv6Hdr(ipv6Hdr)
	if err != nil {
		return err
	}

	// Validating icmpv6 header
	ndInfo, err := p.decodeICMPv6Hdr(icmpv6Hdr, ipv6Hdr.SrcIP, ipv6Hdr.DstIP)
	if err != nil {
		return err
	}

	// Validating checksum received
	err = validateChecksum(ipv6Hdr.SrcIP, ipv6Hdr.DstIP, icmpv6Hdr)
	if err != nil {
		return err
	}

	// Populate Neighbor Information
	p.populateNeighborInfo(nbrInfo, eth, ipv6Hdr, icmpv6Hdr, ndInfo)
	return nil
}
