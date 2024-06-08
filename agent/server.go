package agent

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/poolpOrg/go-agentbuilder/protocol"
	"github.com/poolpOrg/go-agentbuilder/server"
)

type Session struct {
	PublicKey       ed25519.PublicKey
	OperatingSystem string
	Architecture    string
	Hostname        string
	NumCPU          int
}

func NewSession() *Session {
	return &Session{}
}

type Server struct {
	server     *server.Server
	publicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey

	sessions      map[string]*Session
	sessionsMutex sync.Mutex
}

func NewServer(address string, publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey) *Server {
	return &Server{
		server:     server.NewServer(address),
		publicKey:  publicKey,
		privateKey: privateKey,

		sessions: make(map[string]*Session),
	}
}

func (s *Server) ListenAndServe() {
	s.server.ListenAndServe(s.serverHandler)
}

func (s *Server) serverHandler(session *server.Session, incoming <-chan protocol.Packet) error {
	clientSession := NewSession()
	defer func() {
		s.sessionsMutex.Lock()
		if clientSession.PublicKey != nil {
			delete(s.sessions, base64.RawStdEncoding.EncodeToString(clientSession.PublicKey))
		}
		s.sessionsMutex.Unlock()
	}()

	err := session.Query(NewReqIdentify(s.publicKey), func(response interface{}) error {
		if res, ok := response.(ResIdentify); ok {
			clientSession.PublicKey = res.PublicKey
			clientSession.OperatingSystem = res.OperatingSystem
			clientSession.Architecture = res.Architecture
			clientSession.Hostname = res.Hostname
			clientSession.NumCPU = res.NumCPU

			s.sessionsMutex.Lock()
			s.sessions[base64.RawStdEncoding.EncodeToString(clientSession.PublicKey)] = clientSession
			s.sessionsMutex.Unlock()

			fmt.Println("client identified as", base64.RawStdEncoding.EncodeToString(res.PublicKey))
			return nil
		} else {
			return fmt.Errorf("unexpected response type: %T", response)
		}
	})
	if err != nil {
		return err
	}

	err = session.Query(NewReqPushConfiguration(), func(response interface{}) error {
		switch res := response.(type) {
		case ResOK:
			fmt.Println("client configuration pushed successfully")
			return nil
		case ResKO:
			return fmt.Errorf("client configuration push failed: %s", res.Err)
		default:
			return fmt.Errorf("unexpected response type: %T", response)
		}
	})
	if err != nil {
		return err
	}

	for {
		select {
		case packet, ok := <-incoming:
			if !ok {
				return nil
			}
			switch req := packet.Payload.(type) {
			default:
				return fmt.Errorf("unexpected request type: %T", req)
			}

		case <-time.After(60 * time.Second):
			fmt.Println("no packets received in 60 seconds")
		}
	}
	return nil
}
