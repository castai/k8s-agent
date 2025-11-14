package shutdown

import "errors"

func RunThenTrigger(trigger Trigger, skipOnSuccess bool, fn func() error) {
	var (
		err      error
		finished bool
	)
	defer func() {
		if !finished {
			// In case of a panic, we don't want to lose the original stacktrace. Giving a clear reason is enough. Panic
			// should get logged upstream anyway.
			err = errors.New("panic")
		}
		if err != nil || !skipOnSuccess {
			trigger(err)
		}
	}()
	err = fn()
	finished = true
}
