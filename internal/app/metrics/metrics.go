// SPDX-License-Identifier: GPL-3.0-or-later

package metrics

import "sync/atomic"

// Metrics 是进程内的运行计数，只增不减。
type Metrics struct {
	eventsReceived atomic.Int64
	actionsHandled atomic.Int64
}

// New 创建计数集合。
func New() *Metrics {
	return &Metrics{}
}

// EventReceived 记录一次事件。
func (m *Metrics) EventReceived() {
	if m != nil {
		m.eventsReceived.Add(1)
	}
}

// ActionHandled 记录一次框架调用。
func (m *Metrics) ActionHandled() {
	if m != nil {
		m.actionsHandled.Add(1)
	}
}

// EventsReceived 返回已处理的事件数量。
func (m *Metrics) EventsReceived() int64 {
	if m == nil {
		return 0
	}
	return m.eventsReceived.Load()
}

// ActionsHandled 返回已处理的调用数量。
func (m *Metrics) ActionsHandled() int64 {
	if m == nil {
		return 0
	}
	return m.actionsHandled.Load()
}
