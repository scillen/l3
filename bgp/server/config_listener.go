// server.go
package server

import (
    "fmt"
    "net"
	"net/rpc"
)

type GlobalConfigAttrs struct {
    AS int
}

type PeerConfigAttrs struct {
    IP net.IP
    AS int
}

type PeerConfigCommands struct {
    IP net.IP
    Command int
}

type ConfigInterface struct {
    GlobalConfigCh chan GlobalConfigAttrs
    AddPeerConfigCh chan PeerConfigAttrs
    RemPeerConfigCh chan PeerConfigAttrs
    PeerCommandCh chan PeerConfigCommands
}

func (confIface *ConfigInterface) SetBGPConfig(in *GlobalConfigAttrs, out *bool) error {
    confIface.GlobalConfigCh <- *in
    fmt.Println("Got global config attrs:", in)
    *out = true
    return nil
}

func (confIface *ConfigInterface) AddPeer(in *PeerConfigAttrs, out *bool) error {
    confIface.AddPeerConfigCh <- *in
    fmt.Println("Got add peer attrs:", in)
    *out = true
    return nil
}

func (confIface *ConfigInterface) RemovePeer(in *PeerConfigAttrs, out *bool) error {
    confIface.RemPeerConfigCh <- *in
    fmt.Println("Got add peer attrs:", in)
    *out = true
    return nil
}

func (confIface *ConfigInterface) PeerCommand(in *PeerConfigCommands, out *bool) error {
    confIface.PeerCommandCh <- *in
    fmt.Println("Good peer command:", in)
    *out = true
    return nil
}

func NewConfigInterface() *ConfigInterface {
    confIface := new(ConfigInterface)
    confIface.GlobalConfigCh = make(chan GlobalConfigAttrs)
    confIface.AddPeerConfigCh = make(chan PeerConfigAttrs)
    confIface.RemPeerConfigCh = make(chan PeerConfigAttrs)
    confIface.PeerCommandCh = make(chan PeerConfigCommands)
    return confIface
}

func StartConfigListener(conf *ConfigInterface, ip string, port string) error {
    fmt.Printf("Register BGP client interface ip: %s, port: %s\n", ip, port)
    rpc.Register(conf)

    tcpAddr, err := net.ResolveTCPAddr("tcp", ip + ":" + port)
    if err != nil {
        fmt.Println("ResolveTCPAddr failed with", err)
    }

    listener, err := net.ListenTCP("tcp", tcpAddr)
    if err != nil {
        fmt.Println("Listen failed with error", err)
        return err
    }
    rpc.Accept(listener)
    return nil
}