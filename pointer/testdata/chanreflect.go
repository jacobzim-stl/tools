// +build ignore

package main

import "reflect"

//
// This test is very sensitive to line-number perturbations!

// Test of channels with reflection.

var a, b int

func chanreflect1() {
	ch := make(chan *int, 0)
	crv := reflect.ValueOf(ch)
	crv.Send(reflect.ValueOf(&a))
	print(crv.Interface())             // @concrete chan *int
	print(crv.Interface().(chan *int)) // @pointsto makechan@testdata/chanreflect.go:15:12
	print(<-ch)                        // @pointsto main.a
}

func chanreflect2() {
	ch := make(chan *int, 0)
	ch <- &b
	crv := reflect.ValueOf(ch)
	r, _ := crv.Recv()
	print(r.Interface())        // @concrete *int
	print(r.Interface().(*int)) // @pointsto main.b
}

func chanOfRecv() {
	// MakeChan(<-chan) is a no-op.
	t := reflect.ChanOf(reflect.RecvDir, reflect.TypeOf(&a))
	print(reflect.Zero(t).Interface())                      // @concrete <-chan *int
	print(reflect.MakeChan(t, 0).Interface().(<-chan *int)) // @pointsto
}

func chanOfSend() {
	// MakeChan(chan<-) is a no-op.
	t := reflect.ChanOf(reflect.SendDir, reflect.TypeOf(&a))
	print(reflect.Zero(t).Interface())                      // @concrete chan<- *int
	print(reflect.MakeChan(t, 0).Interface().(chan<- *int)) // @pointsto
}

func chanOfBoth() {
	t := reflect.ChanOf(reflect.BothDir, reflect.TypeOf(&a))
	print(reflect.Zero(t).Interface()) // @concrete chan *int
	ch := reflect.MakeChan(t, 0)
	print(ch.Interface().(chan *int)) // @pointsto reflectMakechan@testdata/chanreflect.go:49:24
	ch.Send(reflect.ValueOf(&b))
	ch.Interface().(chan *int) <- &a
	r, _ := ch.Recv()
	print(r.Interface().(*int))         // @pointsto main.a | main.b
	print(<-ch.Interface().(chan *int)) // @pointsto main.a | main.b
}

var unknownDir reflect.ChanDir // not a constant

func chanOfUnknown() {
	// Unknown channel direction: assume all three.
	// MakeChan only works on the bi-di channel type.
	t := reflect.ChanOf(unknownDir, reflect.TypeOf(&a))
	print(reflect.Zero(t).Interface())        // @concrete <-chan *int | chan<- *int | chan *int
	print(reflect.MakeChan(t, 0).Interface()) // @concrete chan *int
}

func main() {
	chanreflect1()
	chanreflect2()
	chanOfRecv()
	chanOfSend()
	chanOfBoth()
	chanOfUnknown()
}
