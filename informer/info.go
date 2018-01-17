package main

import (
	"time"
	"fmt"
	"bufio"
	"os"
	"strings"

	"github.com/go-errors/errors"

	"gomfc/ws_client"
	"gomfc/models"

)

const waitTimeout = 60 * time.Second


func getModelUid(modelName string) (uid uint64, err error) {
	wsConn, err := ws_client.CreateConnection(modelName)
	if err != nil {
		return
	}
	modelRaw, err := wsConn.WaitData(waitTimeout)
	if err != nil {
		return
	}
	model, err := models.GetModelData(modelRaw)
	if err != nil {
		return
	}
	uid = model.Uid
	return
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
	modelUid, err := getModelUid(modelName)
	if err != nil {
		if err == models.NotFoundError {
			fmt.Println("Can't find model")
			return
		} else {
			panic(err)
		}
	}
	fmt.Printf("Model uid: %d\n", modelUid)
}

