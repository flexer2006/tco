package adapters

type atomicWriter struct {
	sharedAtomicWriter
}

func newAtomicWriter() atomicWriter {
	return atomicWriter{sharedAtomicWriter: newSharedAtomicWriter()}
}
