package ws_client

import (
	"fmt"
	"time"
	"sync"

	"math/rand"
	"net/http"
	"net/url"

	"io/ioutil"

	"golang.org/x/net/websocket"

	"encoding/json"

	"strings"
	"strconv"
	"errors"
)

const wsHostPattern = "wss://%s.myfreecams.com/fcsl"
const serverCfgUrl = "https://www.myfreecams.com/_js/serverconfig.js"
const apiChallengePattern = "https://api.myfreecams.com/dc?nc=%.16f&site=%s"
const site = "www"
const intervalLen = 600
const sessionPosition  = 5
const tokenPosition = 2
const wsPingTimeout = 10 * time.Second
const maxTries = 3

type ApiChallengeResult struct {
	Id string
	ResponseVer uint8
	Method string
	Result struct{
		Time int64
		Cid string
		Data string
		Key string
	}
	Err int64
}

func GetApiChallengeResult() (apiChallengResponse *ApiChallengeResult, err error) {
	apiUrl := fmt.Sprintf(apiChallengePattern, rand.Float64(), site)
	resp, err := http.Get(apiUrl)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	err = json.Unmarshal(body, &apiChallengResponse)
	return
}

type WSConnector struct {
	modelName      string
	sync.Mutex
	*websocket.Conn
	result         chan string
	stop           chan struct{}
	tokenId        string
	sessionId      string
	modelRequestId int64

	err   error
	trace string
}

func getXChatServer() (xchat string, err error) {
	type CfgResp struct {
		Chat_Servers []string
	}
	resp, err := http.Get(serverCfgUrl)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	cfgResp := CfgResp{}
	err = json.Unmarshal(body, &cfgResp)
	if err != nil {
		return
	}
	xchat = cfgResp.Chat_Servers[0]
	return
}

func (c *WSConnector) GetTokenId() string {
	return c.tokenId
}

func (c *WSConnector) SendString(message string) error {
	c.Lock()
	defer c.Unlock()
	if _, err := c.Write([]byte(message)); err != nil {
		return err
	}
	return nil
}

func (c *WSConnector) Close() error {
	c.stop <- struct{}{}
	close(c.stop)
	return c.Conn.Close()
}

func CreateConnection(modelName string) (ws WSConnector, err error) {
	var tries = 0
	Start:
	tries++
	if tries > maxTries {
		err = errors.New("websocket error, try again")
		return
	}
	ws.modelName = modelName
	challengeResult, err := GetApiChallengeResult()
	if err != nil {
		return
	}
	cid, key, timeR := challengeResult.Result.Cid, challengeResult.Result.Key, challengeResult.Result.Time
	xchat, err := getXChatServer()
	if err != nil {
		return
	}
	wsUrl := fmt.Sprintf(wsHostPattern, xchat)
	origin := "http://localhost/"
	ws.Conn, err = websocket.Dial(wsUrl, "", origin)
	if err != nil {
		return
	}
	if err = ws.SendString("hello fcserver\n"); err != nil {
		return
	}
	requestData := struct {
		Err   int    `json:"err"`
		Start int64  `json:"start"`
		Stop  int64  `json:"stop"`
		A     int64  `json:"a"`
		Time  int64  `json:"time"`
		Key   string `json:"key"`
		Cid   string `json:"cid"`
		Pid   int    `json:"pid"`
		Site  string `json:"site"`
	}{
		Err:   0,
		Start: time.Now().UnixNano() / int64(time.Millisecond),
		Stop:  time.Now().UnixNano() / int64(time.Millisecond) + intervalLen,
		A:     0,
		Time:  timeR,
		Key:   key,
		Cid:   cid,
		Pid:   1,
		Site:  site,
	}
	requestDataStr, err := json.Marshal(requestData)
	if err != nil {
		return
	}
	escapedRequestDataStr := url.QueryEscape(string(requestDataStr))
	if ws.SendString("1 0 0 81 0 " + escapedRequestDataStr + "\n"); err != nil {
		return
	}
	if err = ws.SendString("0 0 0 0 0\n"); err != nil {
		return
	}
	ws.result = make(chan string)
	ws.stop = make(chan struct{})
	var splited []string
	respMsg := ""
	err = websocket.Message.Receive(ws.Conn, &respMsg)
	if err != nil {
		return
	}
	splited = strings.Fields(respMsg)
	if len(splited) < tokenPosition {
		goto Start
	}
	ws.tokenId = splited[tokenPosition]
	err = websocket.Message.Receive(ws.Conn, &respMsg)
	if err != nil {
		return
	}
	splited = strings.Fields(respMsg)
	if len(splited) < sessionPosition {
		goto Start
	}
	ws.sessionId = splited[sessionPosition]
	if err = ws.SendString(fmt.Sprintf("1 0 0 20071025 0 %s@1/guest:guest\n", ws.sessionId)); err != nil {
		return
	}
	if ws.modelName != "" {
		ws.modelRequestId = time.Now().UnixNano() / 1000000000
		modelRequest := fmt.Sprintf("10 %s 0 %d 0 %s\n", ws.tokenId, ws.modelRequestId, ws.modelName)
		err = ws.SendString(modelRequest)
		if err != nil {
			return
		}
		err = ws.SendString(fmt.Sprintf("44 %s 0 1 0\n", ws.tokenId))
		if err != nil {
			return
		}
	}
	go ws.Serve()
	return
}

func (c *WSConnector) Serve() {
	var respMsg string
	var err error
	defer func() {
		close(c.result)
	}()
	defer func() {
		if err != nil {
			c.err = err
		}
	}()
	senderQuitChan := make(chan struct{})
	defer func() {
		senderQuitChan <- struct{}{}
		close(senderQuitChan)
	}()
	go func(){
		ticker := time.NewTicker(wsPingTimeout)
		for {
			select {
			case <-senderQuitChan:
				return
			case <-ticker.C:
				if err = c.SendString("0 0 0 0 0\n"); err != nil {
					return
				}
			}
		}
	}()
	found := false
	for {
		select {
		case <-c.stop:
			return
		default:
			err = websocket.Message.Receive(c.Conn, &respMsg)
			if err != nil {
				return
			}
			if !found && strings.Contains(respMsg, strconv.FormatInt(c.modelRequestId, 10)) {
				found = true
				c.result <- respMsg
			}
		}
	}
}

func (c *WSConnector) 	WaitData(timeout time.Duration) (result string, err error) {
	ServeLoop:
	for {
		select {
		case msg, ok := <-c.result:
			if !ok {
				if c.err != nil {
					err = c.err
				}
			} else {
				result = msg
			}
			break ServeLoop
		case <-time.After(timeout):
			err = errors.New("response timeout")
			break ServeLoop
		}
	}
	return
}