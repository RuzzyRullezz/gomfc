package models

import (
	"regexp"
	"net/url"
	"encoding/json"
)

const HDFlag int32 = 1024

const (
	IsOnline  uint64 = 0
	Except    uint64 = 90
	IsOff     uint64 = 127
	IsAway    uint64 = 2
	IsPrivate uint64 = 12
	IsGroup   uint64 = 13
)

var StatusVerbose = map[uint64]string{
	IsOnline:  "online",
	Except:    "off (vs=90)",
	IsOff:     "off",
	IsAway:    "away",
	IsPrivate: "in private",
	IsGroup:   "in group show",
}

type MFCModel struct {
	Lv int
	Nm string
	Pid int64
	Sid uint64
	Uid uint64
	Vs uint64
	U struct{
		Camserv int32
	}
	M struct{
		Flags int32
	}
	Exists bool
	Status string
}

func (m *MFCModel) SetStatus() {
	if !m.Exists {
		m.Status = "unknown model"
		return
	}
	verbose, ok := StatusVerbose[m.Vs]
	if !ok {
		m.Status = "unknowm status"
		return
	}
	m.Status = verbose
	return
}

func (m *MFCModel) RecordEnable() bool {
	return m.Vs == IsOnline
}

func (m *MFCModel) IsHD() bool {
	return m.M.Flags & HDFlag != 0
}

func GetModelData(raw string) (mfcmodel MFCModel, err error) {
	defer func() {
		mfcmodel.Exists = mfcmodel.Nm != ""
		mfcmodel.SetStatus()
	}()
	var result string
	result, err = url.QueryUnescape(raw)
	if err != nil {
		return
	}
	pattern := regexp.MustCompile(`\{.*\}`)
    result = pattern.FindString(result)
	if err = json.Unmarshal([]byte(result), &mfcmodel); err != nil {
		return
	}
    return
}