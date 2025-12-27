package ds

type TokenBucket interface {
	Try() bool
	Get()
}
