mod engine;

use std::ffi::{c_char, c_int, CStr, CString};
use std::sync::{Mutex, OnceLock};

use engine::Engine;

// ---------------------------------------------------------------------------
// Global singleton
// ---------------------------------------------------------------------------

static ENGINE: OnceLock<Mutex<Engine>> = OnceLock::new();

/// Obtain a reference to the global `OnceLock<Mutex<Engine>>`.
/// Returns `None` if `vecstore_init` has not been called yet.
fn get_engine() -> Option<&'static Mutex<Engine>> {
    ENGINE.get()
}

// ---------------------------------------------------------------------------
// Original ping / version / free_string functions (must remain)
// ---------------------------------------------------------------------------

/// Health check — returns 1 if the engine is operational.
#[unsafe(no_mangle)]
pub extern "C" fn vecstore_ping() -> i32 {
    1
}

/// Returns the engine version as a C string. Caller must free with vecstore_free_string.
#[unsafe(no_mangle)]
pub extern "C" fn vecstore_version() -> *mut c_char {
    let version = CString::new(env!("CARGO_PKG_VERSION")).unwrap();
    version.into_raw()
}

/// Free a string allocated by vecstore.
///
/// # Safety
/// `s` must be a pointer previously returned by `vecstore_version`, or NULL.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn vecstore_free_string(s: *mut c_char) {
    if s.is_null() {
        return;
    }
    unsafe {
        let _ = CString::from_raw(s);
    }
}

// ---------------------------------------------------------------------------
// New C-ABI functions
// ---------------------------------------------------------------------------

/// Initialise the global Engine with the given data directory.
///
/// Returns 0 on success (including when already initialised — the call is
/// idempotent) or -2 on initialisation failure.
///
/// # Safety
/// `data_dir` must be a valid, null-terminated UTF-8 C string.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn vecstore_init(data_dir: *const c_char) -> c_int {
    if data_dir.is_null() {
        return -2;
    }

    let dir = match unsafe { CStr::from_ptr(data_dir) }.to_str() {
        Ok(s) => s,
        Err(_) => return -2,
    };

    match Engine::new(dir) {
        Ok(engine) => {
            // OnceLock::set fails only if already initialised; treat as success
            // so that calling vecstore_init more than once is idempotent.
            let _ = ENGINE.set(Mutex::new(engine));
            0
        }
        Err(_) => -2,
    }
}

/// Embed `text` into a float vector.  The caller provides `out` (a
/// pre-allocated `f32` buffer of length `out_len`) and `out_len`.
///
/// Returns the number of dimensions written on success (384), or a
/// negative error code.
///
/// # Safety
/// `text` must be a valid, null-terminated UTF-8 C string.
/// `out` must be a valid pointer to at least `out_len` f32 values.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn vecstore_embed(
    text: *const c_char,
    out: *mut f32,
    out_len: c_int,
) -> c_int {
    if text.is_null() || out.is_null() || out_len <= 0 {
        return -1;
    }

    let text_str = match unsafe { CStr::from_ptr(text) }.to_str() {
        Ok(s) => s,
        Err(_) => return -1,
    };

    let engine_lock = match get_engine() {
        Some(e) => e,
        None => return -2, // not initialised
    };

    let engine = match engine_lock.lock() {
        Ok(e) => e,
        Err(_) => return -3, // poisoned
    };

    let vec = match engine.embed(text_str) {
        Ok(v) => v,
        Err(_) => return -4,
    };

    let n = vec.len().min(out_len as usize);
    unsafe {
        std::ptr::copy_nonoverlapping(vec.as_ptr(), out, n);
    }

    n as c_int
}

/// Add a vector with string `id` to the index.
///
/// Returns 0 on success, negative on failure.
///
/// # Safety
/// `id` must be a valid, null-terminated UTF-8 C string.
/// `vector` must point to at least `dim` f32 values.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn vecstore_add(id: *const c_char, vector: *const f32, dim: c_int) -> c_int {
    if id.is_null() || vector.is_null() || dim <= 0 {
        return -1;
    }

    let id_str = match unsafe { CStr::from_ptr(id) }.to_str() {
        Ok(s) => s,
        Err(_) => return -1,
    };

    let vec_slice = unsafe { std::slice::from_raw_parts(vector, dim as usize) };

    let engine_lock = match get_engine() {
        Some(e) => e,
        None => return -2,
    };

    let mut engine = match engine_lock.lock() {
        Ok(e) => e,
        Err(_) => return -3,
    };

    match engine.add(id_str, vec_slice) {
        Ok(_) => 0,
        Err(_) => -4,
    }
}

/// Search for `k` nearest neighbours of `query`.
///
/// On success, `*out_ids` is set to a newly allocated array of `*mut c_char`
/// and `*out_scores` is set to a newly allocated `f32` array; both must be
/// freed together by passing them to `vecstore_free_results`.  The function
/// returns the number of results (≥ 0).
///
/// On failure both out-pointers are set to NULL and a negative error code is
/// returned.
///
/// # Safety
/// `query` must point to at least `dim` f32 values.
/// `out_ids` and `out_scores` must be valid non-null double pointers.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn vecstore_search(
    query: *const f32,
    dim: c_int,
    k: c_int,
    out_ids: *mut *mut *mut c_char,
    out_scores: *mut *mut f32,
) -> c_int {
    if query.is_null() || dim <= 0 || k <= 0 || out_ids.is_null() || out_scores.is_null() {
        return -1;
    }

    // Initialise out-pointers to NULL so that any early-return error path
    // leaves them in a defined, safe state for the caller.
    unsafe {
        *out_ids = std::ptr::null_mut();
        *out_scores = std::ptr::null_mut();
    }

    let query_slice = unsafe { std::slice::from_raw_parts(query, dim as usize) };

    let engine_lock = match get_engine() {
        Some(e) => e,
        None => return -2,
    };

    let engine = match engine_lock.lock() {
        Ok(e) => e,
        Err(_) => return -3,
    };

    let results = match engine.search(query_slice, k as usize) {
        Ok(r) => r,
        Err(_) => return -4,
    };

    let count = results.len();

    // Allocate arrays for ids and scores.
    let mut ids_vec: Vec<*mut c_char> = Vec::with_capacity(count);
    let mut scores_vec: Vec<f32> = Vec::with_capacity(count);

    for (id, score) in results {
        let c_str = match CString::new(id) {
            Ok(s) => s,
            Err(_) => return -5,
        };
        ids_vec.push(c_str.into_raw());
        scores_vec.push(score);
    }

    // Transfer ownership of the arrays to the caller.
    ids_vec.shrink_to_fit();
    scores_vec.shrink_to_fit();

    let ids_ptr = ids_vec.as_mut_ptr();
    let scores_ptr = scores_vec.as_mut_ptr();

    // Prevent Rust from dropping the data.
    std::mem::forget(ids_vec);
    std::mem::forget(scores_vec);

    unsafe {
        *out_ids = ids_ptr;
        *out_scores = scores_ptr;
    }

    count as c_int
}

/// Delete the vector with the given `id` from the index.
///
/// Returns 0 on success, negative on failure.
///
/// # Safety
/// `id` must be a valid, null-terminated UTF-8 C string.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn vecstore_delete(id: *const c_char) -> c_int {
    if id.is_null() {
        return -1;
    }

    let id_str = match unsafe { CStr::from_ptr(id) }.to_str() {
        Ok(s) => s,
        Err(_) => return -1,
    };

    let engine_lock = match get_engine() {
        Some(e) => e,
        None => return -2,
    };

    let mut engine = match engine_lock.lock() {
        Ok(e) => e,
        Err(_) => return -3,
    };

    match engine.delete(id_str) {
        Ok(_) => 0,
        Err(_) => -4,
    }
}

/// Persist the index to disk.
///
/// Returns 0 on success, negative on failure.
#[unsafe(no_mangle)]
pub extern "C" fn vecstore_save() -> c_int {
    let engine_lock = match get_engine() {
        Some(e) => e,
        None => return -2,
    };

    let engine = match engine_lock.lock() {
        Ok(e) => e,
        Err(_) => return -3,
    };

    match engine.save() {
        Ok(_) => 0,
        Err(_) => -4,
    }
}

/// Return the embedding dimension (384).
///
/// Returns the dimension on success, or -2 if the engine is not initialised.
#[unsafe(no_mangle)]
pub extern "C" fn vecstore_dimension() -> c_int {
    let engine_lock = match get_engine() {
        Some(e) => e,
        None => return -2,
    };

    let engine = match engine_lock.lock() {
        Ok(e) => e,
        Err(_) => return -3,
    };

    engine.dimension() as c_int
}

/// Free the arrays returned by `vecstore_search`.
///
/// Both `ids` (the array of C strings) and `scores` (the f32 array) must be
/// the pointers written by `vecstore_search`, and `count` must match the
/// return value of that call.  After this call `ids`, all C strings it
/// pointed to, and `scores` are invalidated.
///
/// # Safety
/// `ids` and `scores` must be the pointers returned by `vecstore_search` and
/// `count` must match the return value of that call.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn vecstore_free_results(
    ids: *mut *mut c_char,
    scores: *mut f32,
    count: c_int,
) {
    if count <= 0 {
        return;
    }

    if !ids.is_null() {
        let slice = unsafe { std::slice::from_raw_parts_mut(ids, count as usize) };
        for ptr in slice.iter() {
            if !ptr.is_null() {
                unsafe {
                    let _ = CString::from_raw(*ptr);
                }
            }
        }

        // Rebuild the Vec so Rust drops (deallocates) the outer ids array.
        // Safety: capacity == len is guaranteed because shrink_to_fit() was
        // called on ids_vec before as_mut_ptr() + mem::forget() in
        // vecstore_search.
        let _ = unsafe { Vec::from_raw_parts(ids, count as usize, count as usize) };
    }

    if !scores.is_null() {
        // Safety: capacity == len is guaranteed because shrink_to_fit() was
        // called on scores_vec before as_mut_ptr() + mem::forget() in
        // vecstore_search.
        let _ = unsafe { Vec::from_raw_parts(scores, count as usize, count as usize) };
    }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_ping() {
        assert_eq!(vecstore_ping(), 1);
    }

    #[test]
    fn test_version() {
        let ptr = vecstore_version();
        assert!(!ptr.is_null());
        let version = unsafe { CStr::from_ptr(ptr) }.to_str().unwrap();
        assert_eq!(version, env!("CARGO_PKG_VERSION"));
        unsafe { vecstore_free_string(ptr) };
    }
}
