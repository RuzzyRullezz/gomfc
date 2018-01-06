package rtmpdump

import (
	"fmt"
	"strconv"
	"strings"
	"regexp"
	"log"
	"bytes"
	"time"
	"errors"

	rtmp "gomfc/gortmp"
	"gomfc/models"

	"github.com/dop251/goja"
	"github.com/zhangpeihao/goflv"
	"github.com/zhangpeihao/goamf"
)

const gType = "DOWNLOAD"
const rtmpPattern = "rtmp://video%d.myfreecams.com:1935/NxServer"
const playpathPattern = "mp4:mfc_%d.f4v"
const playpathPatternPure = "mfc_%d"
const serverOffset = -500
const serverOffsetPure = -34
const roomOffset = 100000000
const loginResultCMD = "loginResult"
const chanReadyTimeout = 60 * time.Second
const dataReceiveTimeout = 30 * time.Second

var jsRegexp = regexp.MustCompile(`\(function\(.+\)`)
var streamReadyChan = make(chan struct{})
var streamCloseChan = make(chan struct{})
var dataSize int64

type RtmpConn struct {
	ServerUrl string
	Playpath string
	SessionId uint64
	ModelId uint64
	RoomId uint64
}

func RtmpUrlData(m *models.MFCModel) (rtmpConnData *RtmpConn) {
	var serverId int32
	var playPath string
	if m.U.Camserv + serverOffset > 0 {
		serverId = m.U.Camserv + serverOffset
	} else {
		serverId = m.U.Camserv + serverOffsetPure
	}
	if m.IsHD() {
		playPath = playpathPattern
	} else {
		playPath = playpathPatternPure
	}
	rtmpConnData = &RtmpConn{
		ServerUrl: fmt.Sprintf(rtmpPattern, serverId),
		SessionId: m.Sid,
		ModelId: m.Uid,
		RoomId: m.Uid + roomOffset,
		Playpath: fmt.Sprintf(playPath, m.Uid + roomOffset,),
	}
	return
}

type MfcRtmpHandler struct {
	*flv.File
	OutBountStreamChan chan rtmp.OutboundStream
}

func (handler *MfcRtmpHandler) OnStatus(conn rtmp.OutboundConn) {}

func (handler *MfcRtmpHandler) OnClosed(conn rtmp.Conn) {
	streamCloseChan <- struct{}{}
}

func (handler *MfcRtmpHandler) OnReceived(conn rtmp.Conn, message *rtmp.Message) {
	msgAsString := message.Buf.String()
	if strings.Contains(msgAsString, loginResultCMD) {
		num, err := amf.ReadDouble(bytes.NewReader(message.Buf.Bytes()[14:23]))
		if err != nil {
			log.Fatal(err)
		}
		jsCode := jsRegexp.FindString(msgAsString)
		jsCode = strings.Replace(jsCode, "!!screen.width", "1", 1)
		jsCode = strings.Replace(jsCode, "!!screen.height", "1", 1)
		jsCode = strings.Replace(jsCode, "!!document.location.host", "1", 1)
		vm := goja.New()
		v, err := vm.RunString(jsCode)
		if err != nil {
			log.Fatal(err)
		}
		challengeResult := v.ToString()

		buf := new(bytes.Buffer)
		cmd := &rtmp.Command{
			IsFlex:        false,
			Name:          "_result",
			TransactionID: uint32(num),
			Objects: []interface{}{nil, challengeResult},
		}
		err = cmd.Write(buf)
		if err != nil {
			log.Fatal(err)
		}
		msg := rtmp.NewMessage(rtmp.CS_ID_COMMAND, rtmp.COMMAND_AMF0, 0, 0, buf.Bytes())
		conn.Send(msg)
		streamReadyChan <- struct{}{}
	}
	switch message.Type {
	case rtmp.VIDEO_TYPE:
		if handler.File != nil {
			handler.WriteVideoTag(message.Buf.Bytes(), message.AbsoluteTimestamp)
		}
		dataSize += int64(message.Buf.Len())
	case rtmp.AUDIO_TYPE:
		if handler.File != nil {
			handler.WriteAudioTag(message.Buf.Bytes(), message.AbsoluteTimestamp)
		}
		dataSize += int64(message.Buf.Len())
	}
}

func (handler *MfcRtmpHandler) OnReceivedRtmpCommand(conn rtmp.Conn, command *rtmp.Command) {}

func (handler *MfcRtmpHandler) OnStreamCreated(conn rtmp.OutboundConn, stream rtmp.OutboundStream) {
	handler.OutBountStreamChan <- stream
}

func waitForCreateStreamReady(timeout time.Duration) (err error) {
	select {
	case _, ok := <-streamReadyChan:
		if !ok {
			err = errors.New("streamReadyChan closed")
		}
	case <-time.After(timeout):
		err = errors.New("streamReadyChan wait timeout")
	}
	return err
}

func RecordStream(serverUrl string, roomId, modelId int64, playPath string, wsToken string, flv *flv.File) (err error){
	createStreamChan := make(chan rtmp.OutboundStream)
	mfcHandler := &MfcRtmpHandler{
		File:flv,
		OutBountStreamChan: createStreamChan,
	}
	obConn, err := rtmp.Dial(serverUrl, mfcHandler, 100)
	if err != nil {
		return
	}
	defer obConn.Close()
	err = obConn.Connect(wsToken, "", strconv.FormatInt(roomId, 10), gType, modelId, 0, "")
	if err != nil {
		return
	}
	err = waitForCreateStreamReady(chanReadyTimeout)
	if err != nil {
		return
	}
	err = obConn.CreateStream()
	if err != nil {
		return
	}
	select {
	case stream := <-createStreamChan:
		stream.Play(playPath, nil, nil, nil)
	case <-time.After(chanReadyTimeout):
		return errors.New("create stream timeout")
	}
	lastGet := int64(0)
	dataReceiveTicker := time.NewTicker(dataReceiveTimeout)
	everySecond := time.NewTicker(time.Second)
	for {
		select {
		case <- streamCloseChan:
			fmt.Println("\nStream closed")
			return
		case <- dataReceiveTicker.C:
			if lastGet == dataSize {
				fmt.Println("\nNo data anymore, stream close")
				return
			}
			lastGet = dataSize
		case <- everySecond.C:
			fmt.Printf("\rFile size: %.2f MB (%d)", float32(dataSize)/1024/1024, dataSize)
		}
	}
}