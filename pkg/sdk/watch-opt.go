// Copyright 2018 The Operator-SDK Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sdk

// WatchOp wraps all the options for Watch().
type WatchOp struct {
	NumWorkers int
}

// NewWatchOp create a new deafult WatchOp
func NewWatchOp() *WatchOp {
	op := &WatchOp{}
	op.setDefaults()
	return op
}

func (op *WatchOp) applyOpts(opts []WatchOption) {
	for _, opt := range opts {
		opt(op)
	}
}

func (op *WatchOp) setDefaults() {
	if op.NumWorkers == 0 {
		op.NumWorkers = 1
	}
}

// WatchOption configures WatchOp.
type WatchOption func(*WatchOp)

// WithWatchOptions sets the number of workers for the Watch() operation.
func WithWatchOptions(numWorkers int) WatchOption {
	return func(op *WatchOp) {
		op.NumWorkers = numWorkers
	}
}
