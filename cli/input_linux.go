package main

import "os/exec"

func disableInputBuffering() {
	exec.Command("stty", "-F", "/dev/tty", "cbreak").Run()
}

func enableInputBuffering() {
	exec.Command("stty", "-F", "/dev/tty", "cooked").Run()
}
