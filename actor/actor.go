package actor

import (
	"github.com/yzyalfred/cercis/processor"
	"github.com/yzyalfred/cercis/utils"
	"github.com/yzyalfred/cercis/utils/mpsc"
	"sync/atomic"
	"time"
)

type CallIO struct {
	ClienId  uint32
	TargetId uint64

	Buff []byte
}

type Actor struct {
	isStart bool

	eventChan chan uint8

	// msg
	msgChan     chan bool
	msgFlag     int32
	msgQuene    *mpsc.Queue
	msgCallFunc func(uint32, uint64, interface{})

	// timer
	timer         *time.Ticker
	timerCallFunc func()

	// processor
	processor processor.IProcessor
}

func NewActor() *Actor {
	actor := &Actor{}
	actor.Init()
	return actor
}

func (this *Actor) Init() {
	this.isStart = false

	this.eventChan = make(chan uint8)

	this.msgChan = make(chan bool)
	this.msgFlag = 0
	this.msgQuene = mpsc.New()

	// no timer
	this.timer = time.NewTicker(1<<63 - 1*time.Nanosecond)
	this.timerCallFunc = nil

	// pb processor default
	this.processor = processor.NewPBProcessor()
}

func (this *Actor) Start() {
	if !this.isStart {
		go this.run()
		this.isStart = true
	}
}

func (this *Actor) Stop() {
	this.eventChan <- ACTOR_EVENT_CLOSE
}

func (this *Actor) Send(clentId uint32, targetId uint64, buf []byte) {
	this.msgQuene.Push(CallIO{
		ClienId:  clentId,
		TargetId: targetId,
		Buff:     buf,
	})
	if atomic.CompareAndSwapInt32(&this.msgFlag, 0, 1) {
		this.msgChan <- true
	}
}

func (this *Actor) RegisterTimer(duration time.Duration, callFunc func()) {
	this.timer.Stop()
	this.timer = time.NewTicker(duration)
	this.timerCallFunc = callFunc
}

func (this *Actor) SetProcessor(processorType uint8) {
	if processorType == processor.PROCESSOR_TYPE_PB {
		this.processor = processor.NewPBProcessor()
	}
}

func (this *Actor) clear() {
	this.isStart = false

	if this.timer != nil {
		this.timer.Stop()
		this.timerCallFunc = nil
	}
}

func (this *Actor) run() {
	for {
		if !this.loop() {
			break
		}
	}

	this.clear()
}

func (this *Actor) loop() bool {
	defer func() {
		if err := recover(); err != nil {
			utils.TraceCode()
		}
	}()
	select {
	case <-this.msgChan:
		this.consumeMsg()
	case eventId := <-this.eventChan:
		if eventId == ACTOR_EVENT_CLOSE {
			return false
		}
	case <-this.timer.C:
		this.timerCallFunc()
	}

	return true
}

func (this *Actor) RegisterMsg(msgId interface{}, msg interface{}) {
	this.processor.Register(msgId, msg)
}

func (this *Actor) consumeMsg() {
	for data := this.msgQuene.Pop(); data != nil; data = this.msgQuene.Pop() {
		this.handleMsg(data.(CallIO))
	}
	atomic.StoreInt32(&this.msgFlag, 0)
}

func (this *Actor) handleMsg(callIo CallIO) {
	msg, err := this.processor.Unmarshal(callIo.Buff)
	if err != nil {
		this.msgCallFunc(callIo.ClienId, callIo.TargetId, msg)
	}
}