package rtmpdump

import (
	"time"
	"fmt"
	"os"

	"github.com/zhangpeihao/goflv"

	"gomfc/ws_client"
	"gomfc/models"
	"path/filepath"
)
const waitTimeout = 60 * time.Second
const folder = "streams"

func GetParentDir() (parentDir string, err error){
	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	parentDir = filepath.Dir(ex)
	return
}

func GetFLVName(modelName string) string {
	return fmt.Sprintf("%s_%d.flv", modelName, time.Now().Unix())
}

func CreateFolder(parentDir, folderName string) error {
	return os.MkdirAll(filepath.Join(parentDir, folderName), os.ModePerm)
}

func Record(modelName string, outFile string) (err error) {
	wsConn, err := ws_client.CreateConnection(modelName, false)
	if err != nil {
		return
	}
	modelRaw, err := wsConn.ReadSingle(waitTimeout)
	if err != nil {
		return
	}
	wsToken := wsConn.GetTokenId()
	model, err := models.GetModelData(modelRaw)
	if err != nil {
		return
	}
	if !model.RecordEnable() {
		err = models.NoPublicStreams
		return
	}
	var flvPath string
	if outFile == "" {
		var parentDir string
		parentDir, err = GetParentDir()
		if err != nil {
			return
		}
		err = CreateFolder(parentDir, folder)
		if err != nil {
			return
		}
		flvName := GetFLVName(modelName)
		flvPath, err = filepath.Abs(filepath.Join(parentDir, folder, flvName))
		if err != nil {
			return
		}
	} else {
		flvPath = outFile
	}
	flvFile, err := flv.CreateFile(flvPath)
	if err != nil {
		return
	}
	fmt.Printf("Start record %q into file %s\n", modelName, flvPath)
	connData := RtmpUrlData(&model)
	err = RecordStream(
		connData.ServerUrl,
		int64(connData.RoomId),
		int64(connData.ModelId),
		connData.Playpath,
		wsToken,
		flvFile,
	)
	if err != nil {
		return
	}
	return
}