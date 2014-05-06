package main

import "os/exec"

func disableInputBuffering() {
	exec.Command("stty", "-f", "/dev/tty", "cbreak").Run()
}

func enableInputBuffering() {
	exec.Command("stty", "-f", "/dev/tty", "cooked").Run()
}
