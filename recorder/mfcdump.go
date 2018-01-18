package main

import (
	"os"
	"fmt"
	"bufio"
	"strings"

	"github.com/go-errors/errors"

	"gomfc/rtmpdump"
	"gomfc/models"
)

func exitProgram(waitEnter bool) {
	var exitCode = 0
	if r := recover(); r != nil {
		exitCode = -1
		e, _ := r.(error)
		switch e {
		case models.NoPublicStreams, models.NotFoundError:
			fmt.Println(e)
		default:
			fmt.Println("Error:", e)
			fmt.Println(errors.Wrap(e, 2).ErrorStack())
		}
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
		outFile = ""
	}
	err := rtmpdump.Record(modelName, outFile)
	if err != nil {
		panic(err)
	}
}