// Copyright 2018 Schibsted

package main

// CountOutput wraps a SessionOutput with a delta counter.
type CountOutput struct {
	SessionOutput
	counts map[string]int
}

// NewCountOutput wraps a session output as a count output.
func NewCountOutput(wrapped SessionOutput) *CountOutput {
	return &CountOutput{SessionOutput: wrapped}
}

func (out *CountOutput) Write(key string, value interface{}) error {
	switch v := value.(type) {
	case float64:
		if out.counts == nil {
			out.counts = make(map[string]int)
		}
		/* We only support integers. Maybe converting here is a bit unexpected, has
		 * to be clearly documented.
		 */
		value = int(v)
		out.counts[key] += int(v)
		if out.counts[key] == 0 {
			delete(out.counts, key)
		}
	case nil:
		if out.counts[key] != 0 {
			value = -out.counts[key]
		}
		delete(out.counts, key)
	}
	return out.SessionOutput.Write(key, value)
}

// Close undoes all deltas in the countoutut and closes the underlying session
// output.
func (out *CountOutput) Close(proper, lastRef bool) {
	for k, f := range out.counts {
		out.SessionOutput.Write(k, -f)
	}
	out.SessionOutput.Close(proper, lastRef)
}
