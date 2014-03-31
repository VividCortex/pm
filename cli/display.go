package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func getTermSize() (int, int) {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err != nil {
		return 0, 0
	}
	dims := strings.Split(strings.Trim(string(out), "\n"), " ")
	height, _ := strconv.Atoi(dims[0])
	width, _ := strconv.Atoi(dims[1])
	return height, width
}

func clearScreen(keepHist bool) {
	if keepHist {
		fmt.Print("\033[2J\033[;H")
	} else {
		fmt.Print("\033[0;0H")
		height, width := ScreenHeight, ScreenWidth
		blank := ""
		for width > 0 {
			blank += " "
			width--
		}
		for height > 0 {
			fmt.Print(blank)
			height--
		}
		fmt.Print("\033[0;0H")
	}
}
