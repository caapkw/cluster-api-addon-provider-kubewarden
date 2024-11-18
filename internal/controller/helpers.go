package controller

import "time"

const (
	deployToAll            = true
	kubewardenNamespace    = "kubewarden"
	defaultRequeueDuration = 1 * time.Minute
)
