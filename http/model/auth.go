package model

import (
	"github.com/qiaojun2016/basic/http/contextx"
	"log"
)

type Auth struct {
	Token    string `json:"t"`
	DeviceId string `json:"d"`
	Version  int64  `json:"v"`
}

func FromContextAuth(auth2 *contextx.Auth) *Auth {
	if auth2 == nil {
		log.Println("auth2 is nil")
		return nil
	}
	return &Auth{
		Version:  auth2.Version,
		DeviceId: auth2.DeviceId,
		Token:    auth2.Token,
	}
}
