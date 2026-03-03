package mypkg

type EventName int8

// Low
const (
	AckOk EventName = iota
	RegisterOk
)

// High
const (
	SendOk EventName = iota + 10
	DeleteOk
)
