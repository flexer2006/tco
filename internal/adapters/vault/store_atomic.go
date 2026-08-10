package vault

type manifestAtomicWriter struct {
	sharedAtomicWriter
}

func newManifestAtomicWriter() manifestAtomicWriter {
	return manifestAtomicWriter{sharedAtomicWriter: newSharedAtomicWriter()}
}
