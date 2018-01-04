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

	rtmp "go_private/gortmp"
	"github.com/dop251/goja"

	"go_private/models"
	"github.com/zhangpeihao/goflv"
)

const gType = "DOWNLOAD"
const rtmpPattern = "rtmp://video%d.myfreecams.com:1935/NxServer"
const playpathPattern = "mp4:mfc_%d.f4v"
const serverOffser = -500
const roomOffset = 100000000
const loginResultCMD = "loginResult"
const challengeResultToken = 120781448
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
	rtmpConnData = &RtmpConn{
		ServerUrl: fmt.Sprintf(rtmpPattern, m.U.Camserv + serverOffser),
		SessionId: m.Sid,
		ModelId: m.Uid,
		RoomId: m.Uid + roomOffset,
		Playpath: fmt.Sprintf(playpathPattern, m.Uid + roomOffset,),
	}
	return
}

type MfcRtmpHandler struct {
	*flv.File
	OutBountStreamChan chan rtmp.OutboundStream
	LastReceived       int64
}

func (handler *MfcRtmpHandler) OnStatus(conn rtmp.OutboundConn) {}

func (handler *MfcRtmpHandler) OnClosed(conn rtmp.Conn) {
	streamCloseChan <- struct{}{}
}

func (handler *MfcRtmpHandler) OnReceived(conn rtmp.Conn, message *rtmp.Message) {
	msgAsString := message.Buf.String()
	if strings.Contains(msgAsString, loginResultCMD) {
		jsCode := jsRegexp.FindString(msgAsString)
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
			TransactionID: challengeResultToken,
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
		handler.LastReceived = time.Now().Unix()
	case rtmp.AUDIO_TYPE:
		if handler.File != nil {
			handler.WriteAudioTag(message.Buf.Bytes(), message.AbsoluteTimestamp)
		}
		dataSize += int64(message.Buf.Len())
		handler.LastReceived = time.Now().Unix()
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
		LastReceived: 0,
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
	for {
		select {
		case <- streamCloseChan:
			fmt.Println("\nStream closed")
			return
		case <- time.After(1 * time.Second):
			fmt.Printf("\rFile size: %.2f MB", float32(dataSize)/1024/1024)
			if mfcHandler.LastReceived != 0 && time.Duration(time.Now().Unix() - mfcHandler.LastReceived) * time.Second > dataReceiveTimeout {
				fmt.Println("\nNo data anymore, stream close")
				return
			}
		}
	}
}