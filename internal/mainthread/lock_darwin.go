package mainthread

import "runtime"

// Locking during init keeps the main goroutine on the process main thread for
// the life of the program (see runtime.LockOSThread).
func init() { runtime.LockOSThread() }
