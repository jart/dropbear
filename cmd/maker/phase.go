package main

type Phase uint8

const (
	PhaseNormal Phase = iota
	PhaseExitOnly
	PhaseCanceling
	PhaseFlattening
)
