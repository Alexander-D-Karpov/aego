package scriptwait

const ServiceID = "script.waker"

type Waker interface {
	Complete(typ, id uint64)
}
