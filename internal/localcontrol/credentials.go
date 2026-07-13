//go:build linux

package localcontrol

import (
	"context"
	"google.golang.org/grpc/credentials"
	"net"
	"syscall"
)

type PeerInfo struct{ UID, GID uint32 }

func (PeerInfo) AuthType() string { return "unix-peer" }

type Credentials struct{}

func (Credentials) ClientHandshake(context.Context, string, net.Conn) (net.Conn, credentials.AuthInfo, error) {
	panic("client handshake not supported")
}
func (Credentials) ServerHandshake(conn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	raw, err := conn.(*net.UnixConn).SyscallConn()
	if err != nil {
		return nil, nil, err
	}
	var info PeerInfo
	var sockErr error
	err = raw.Control(func(fd uintptr) {
		cred, e := syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
		if e != nil {
			sockErr = e
			return
		}
		info = PeerInfo{UID: cred.Uid, GID: cred.Gid}
	})
	if err != nil {
		return nil, nil, err
	}
	if sockErr != nil {
		return nil, nil, sockErr
	}
	return conn, info, nil
}
func (Credentials) Info() credentials.ProtocolInfo {
	return credentials.ProtocolInfo{SecurityProtocol: "unix-peer"}
}
func (Credentials) Clone() credentials.TransportCredentials { return Credentials{} }
func (Credentials) OverrideServerName(string) error         { return nil }
