package main

import (
	"time"
	"os"
	"fmt"
	"bufio"
	"strings"
	"path/filepath"

	"gomfc/ws_client"
	"gomfc/models"
	"gomfc/rtmpdump"

	"github.com/zhangpeihao/goflv"
)
const waitTimeout = 5 * time.Second
const folder = "streams"

var parentDir string

func GetFLVName(modelName string, modelUid int64) string {
	return fmt.Sprintf("%d_%d_%s.flv", modelUid, time.Now().Unix(), modelName)
}

func CreateFolder(folderName string) error {
	return os.MkdirAll(filepath.Join(parentDir, folderName), os.ModePerm)
}

func init() {
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	parentDir = filepath.Dir(ex)
}

func exitProgram(waitEnter bool) {
	var exitCode = 0
	if r := recover(); r != nil {
		exitCode = -1
		e, _ := r.(error)
		fmt.Println("Error:", e)
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
	var outFile string
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
	if len(os.Args) >= 3 {
		outFile = os.Args[2]
	} else {
		err := CreateFolder(folder)
		if err != nil {
			panic(err)
		}
	}
	wsConn, err := ws_client.CreateConnection(modelName)
	if err != nil {
		panic(err)
	}
	modelRaw, err := wsConn.WaitData(waitTimeout)
	if err != nil {
		panic(err)
	}
	wsToken := wsConn.GetTokenId()
	model, err := models.GetModelData(modelRaw)
	if !model.Exists {
		fmt.Println(fmt.Errorf("%q does not exist", modelName))
		return
	}
	if !model.RecordEnable() {
		fmt.Println(fmt.Errorf("%q has not public streams", modelName))
		return
	}
	var flvPath string
	if outFile == "" {
		flvName := GetFLVName(model.Nm, int64(model.Uid))
		flvPath, err = filepath.Abs(filepath.Join(parentDir, folder, flvName))
		if err != nil {
			panic(err)
		}
	} else {
		flvPath = outFile
	}
	flvFile, err := flv.CreateFile(flvPath)
	fmt.Printf("Start record %q into file %s\n", modelName, flvPath)
	if err != nil {
		panic(err)
	}
	connData := rtmpdump.RtmpUrlData(&model)
	err = rtmpdump.RecordStream(
		connData.ServerUrl,
		int64(connData.RoomId),
		int64(connData.ModelId),
		connData.Playpath,
		wsToken,
		flvFile,
	)
	if err != nil {
		panic(err)
	}
}