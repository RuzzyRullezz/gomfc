package main

import (
	"fmt"
	"bufio"
	"os"
	"strings"
	"time"
	"sync"

	"github.com/go-errors/errors"

	"gomfc/models"
	"gomfc/ws_client"
	"gomfc/rtmpdump"

)

const stateChanCap = 10000

type ModelState struct {
	models.MFCModel
	ChangeStateTime time.Time
}

type ModelMapType struct {
	sync.RWMutex
	Data map[uint64]ModelState
	StateChan chan ModelState
}

func (m *ModelMapType) Get(uid uint64) (state ModelState, ok bool) {
	m.RLock()
	defer m.RUnlock()
	state, ok = m.Data[uid]
	return
}

func (m *ModelMapType) Set(uid uint64, state ModelState) {
	m.Lock()
	defer m.Unlock()
	m.Data[uid] = state
}

func (m *ModelMapType) SendState(state ModelState) (err error) {
	select {
	case m.StateChan <- state:
	default:
		err = errors.New("state channel is blocked")
	}
	return
}

var ModelMap ModelMapType

func stateHandle(modelName string) {
Loop:
	for {
		select {
		case state, ok := <- ModelMap.StateChan:
			if !ok {
				break Loop
			}
			if state.Nm == modelName {
				for {
					actualState, _ := ModelMap.Get(state.Uid)
					if actualState.RecordEnable()  {
						fmt.Printf("%q is available for record\n", modelName)
						rtmpdump.Record(modelName, "")
					} else {
						break
					}
				}
			}
		}
	}
}

func exitProgram(waitEnter bool) {
	var exitCode = 0
	if r := recover(); r != nil {
		exitCode = -1
		e, _ := r.(error)
		fmt.Println("Error:", e)
		fmt.Println(errors.Wrap(e, 2).ErrorStack())

	}
	if waitEnter {
		fmt.Print("Press enter to continue... ")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
	os.Exit(exitCode)
}

func modelMapper(msg string) (err error){
	model, err := models.GetModelData(msg)
	if err == models.ServiceInfoError {
		err = nil
		return
	}
	if err != nil {
		return
	}
	if model.Lv == models.ModelLv {
		newState := ModelState{model, time.Now()}
		oldState, ok := ModelMap.Get(model.Uid)
		if ok {
			if oldState.Vs != newState.Vs {
				err = ModelMap.SendState(newState)
				if err != nil {
					return
				}
			}
		} else {
			err = ModelMap.SendState(newState)
			if err != nil {
				return
			}
		}
		ModelMap.Set(model.Uid, newState)
	}
	return
}

func init() {
	ModelMap = ModelMapType{
		Data: make(map[uint64]ModelState),
		StateChan: make(chan ModelState, stateChanCap),
	}
}

func main() {
	var waitEnter bool
	var modelName string
	if len(os.Args) == 1 {
		waitEnter = true
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Enter model name: ")
		modelName, _ = reader.ReadString('\n')
		modelName = strings.Replace(modelName, "\n", "", 1)
		modelName = strings.Replace(modelName, "\r", "", 1)
	} else {
		waitEnter = false
		modelName = os.Args[1]
	}
	defer exitProgram(waitEnter)

	wsConn, err := ws_client.CreateConnection(modelName, true)
	if err != nil {
		panic(err)
	}
	go stateHandle(modelName)
	wsConn.SetMsgHdlr(modelMapper)
	err = wsConn.ReadForever()
	if err != nil {
		if err == models.NotFoundError {
			fmt.Println(err)

		} else {
			panic(err)
		}
	}
}

