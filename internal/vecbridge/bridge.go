//go:build cgo

package vecbridge

/*
#cgo LDFLAGS: -L${SRCDIR}/../../vecstore/target/release -lvecstore -lm -ldl
#cgo linux LDFLAGS: -Wl,-rpath,$ORIGIN/../lib
#cgo darwin LDFLAGS: -Wl,-rpath,@executable_path/../lib
#cgo CFLAGS: -I${SRCDIR}/../../vecstore
#include "vecstore.h"
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"sort"
	"unsafe"

	"github.com/jxroo/kairos/internal/memory"
)

func Ping() int {
	return int(C.vecstore_ping())
}

func Version() string {
	cstr := C.vecstore_version()
	defer C.vecstore_free_string(cstr)
	return C.GoString(cstr)
}

// Init initializes the global Rust engine with the given data directory.
// The call is idempotent — calling it again with the same path is safe.
func Init(dataDir string) error {
	cdir := C.CString(dataDir)
	defer C.free(unsafe.Pointer(cdir))
	rc := C.vecstore_init(cdir)
	if rc != 0 {
		return fmt.Errorf("vecstore init: error code %d", rc)
	}
	return nil
}

// Embed converts text into a 384-dimensional float32 vector using the Rust
// embedding model. The engine must be initialized with Init before calling.
func Embed(text string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("vecstore embed: empty text")
	}

	const dim = C.EMBEDDING_DIM
	buf := make([]float32, dim)

	ctext := C.CString(text)
	defer C.free(unsafe.Pointer(ctext))

	rc := C.vecstore_embed(ctext, (*C.float)(&buf[0]), C.int(dim))
	if rc < 0 {
		return nil, fmt.Errorf("vecstore embed: error code %d", rc)
	}
	if int(rc) != int(dim) {
		return nil, fmt.Errorf("vecstore embed: expected %d dimensions, wrote %d", dim, rc)
	}
	return buf, nil
}

// Add inserts (or replaces) a vector identified by id into the index.
func Add(id string, vector []float32) error {
	if len(vector) == 0 {
		return fmt.Errorf("vecstore add: empty vector for id %q", id)
	}
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid))

	rc := C.vecstore_add(cid, (*C.float)(&vector[0]), C.int(len(vector)))
	if rc != 0 {
		return fmt.Errorf("vecstore add: error code %d", rc)
	}
	return nil
}

// Search returns up to k nearest neighbors of query from the Rust index.
func Search(query []float32, k int) ([]memory.SearchHit, error) {
	if len(query) == 0 {
		return nil, fmt.Errorf("vecstore search: empty query vector")
	}
	if k <= 0 {
		return nil, nil
	}

	var outIDs **C.char
	var outScores *C.float

	rc := C.vecstore_search(
		(*C.float)(&query[0]),
		C.int(len(query)),
		C.int(k),
		&outIDs,
		&outScores,
	)
	if rc < 0 {
		return nil, fmt.Errorf("vecstore search: error code %d", rc)
	}

	n := int(rc)
	if n == 0 {
		return nil, nil
	}

	// Copy results into Go memory before freeing the C arrays.
	hits := make([]memory.SearchHit, n)

	// Interpret outIDs as a slice of *C.char pointers.
	idSlice := (*[1 << 28]*C.char)(unsafe.Pointer(outIDs))[:n:n]
	// Interpret outScores as a slice of C.float values.
	scoreSlice := (*[1 << 28]C.float)(unsafe.Pointer(outScores))[:n:n]

	for i := 0; i < n; i++ {
		hits[i] = memory.SearchHit{
			ID:    C.GoString(idSlice[i]),
			Score: float32(scoreSlice[i]),
		}
	}

	// Free both arrays together as required by the API contract.
	C.vecstore_free_results(outIDs, outScores, C.int(n))

	// Sort descending by score so callers always receive results in the
	// expected order regardless of the Rust engine's internal ordering.
	sort.Slice(hits, func(i, j int) bool {
		return hits[i].Score > hits[j].Score
	})

	return hits, nil
}

// DeleteVector removes the vector with the given id from the index.
// Returns an error if the id does not exist in the index.
func DeleteVector(id string) error {
	cid := C.CString(id)
	defer C.free(unsafe.Pointer(cid))
	rc := C.vecstore_delete(cid)
	if rc != 0 {
		return fmt.Errorf("vecstore delete: error code %d", rc)
	}
	return nil
}

// Save persists the index to disk.
func Save() error {
	rc := C.vecstore_save()
	if rc != 0 {
		return fmt.Errorf("vecstore save: error code %d", rc)
	}
	return nil
}

// Dimension returns the embedding dimension (384), or -2 if uninitialized.
func Dimension() int {
	return int(C.vecstore_dimension())
}
