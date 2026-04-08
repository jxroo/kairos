use std::collections::HashMap;
use std::path::PathBuf;

use fastembed::{EmbeddingModel, InitOptions, TextEmbedding};
use usearch::{Index, IndexOptions, MetricKind, ScalarKind};

/// Dimension of the AllMiniLM-L6-V2 embedding model.
pub const EMBEDDING_DIM: usize = 384;

enum Embedder {
    Fast(TextEmbedding),
    #[cfg(test)]
    Test(TestEmbedder),
}

impl Embedder {
    fn embed(&self, text: &str) -> Result<Vec<f32>, anyhow::Error> {
        match self {
            Self::Fast(embedder) => {
                let mut results = embedder
                    .embed(vec![text], None)
                    .map_err(|e| anyhow::anyhow!("embedding text: {}", e))?;

                results
                    .pop()
                    .ok_or_else(|| anyhow::anyhow!("embedder returned empty result"))
            }
            #[cfg(test)]
            Self::Test(embedder) => Ok(embedder.embed(text)),
        }
    }
}

#[cfg(test)]
struct TestEmbedder;

#[cfg(test)]
impl TestEmbedder {
    fn embed(&self, text: &str) -> Vec<f32> {
        let mut vec = vec![0.0; EMBEDDING_DIM];

        for (token_pos, token) in text
            .split_whitespace()
            .map(|token| token.to_lowercase())
            .enumerate()
        {
            let mut hash = 1469598103934665603u64;
            for byte in token.bytes() {
                hash ^= byte as u64;
                hash = hash.wrapping_mul(1099511628211);
            }

            let primary = (hash as usize) % EMBEDDING_DIM;
            let secondary = ((hash >> 32) as usize) % EMBEDDING_DIM;
            let weight = 1.0 + (token_pos as f32 * 0.01);

            vec[primary] += weight;
            vec[secondary] += weight * 0.5;
        }

        let norm = vec.iter().map(|value| value * value).sum::<f32>().sqrt();
        if norm > 0.0 {
            for value in &mut vec {
                *value /= norm;
            }
        }

        vec
    }
}

/// Core vector engine providing embedding generation and ANN search.
pub struct Engine {
    embedder: Embedder,
    index: Index,
    id_map: HashMap<String, u64>,
    reverse_map: HashMap<u64, String>,
    next_key: u64,
    dimension: usize,
    index_path: String,
}

impl Engine {
    /// Create a new Engine, initialising the embedder and the vector index.
    ///
    /// `data_dir` is used as the cache directory for fastembed model files and
    /// as the directory where the index is persisted.
    pub fn new(data_dir: &str) -> Result<Self, anyhow::Error> {
        // Ensure the data directory exists.
        std::fs::create_dir_all(data_dir)
            .map_err(|e| anyhow::anyhow!("creating data dir: {}", e))?;

        let cache_dir = PathBuf::from(data_dir);
        let embedder = Embedder::Fast(
            TextEmbedding::try_new(
                InitOptions::new(EmbeddingModel::AllMiniLML6V2)
                    .with_cache_dir(cache_dir)
                    .with_show_download_progress(false),
            )
            .map_err(|e| anyhow::anyhow!("initialising embedder: {}", e))?,
        );

        Self::new_with_embedder(data_dir, embedder)
    }

    fn new_with_embedder(data_dir: &str, embedder: Embedder) -> Result<Self, anyhow::Error> {
        std::fs::create_dir_all(data_dir)
            .map_err(|e| anyhow::anyhow!("creating data dir: {}", e))?;

        let index_path = format!("{}/vecstore.usearch", data_dir);

        // Build usearch index with cosine similarity.
        let options = IndexOptions {
            dimensions: EMBEDDING_DIM,
            metric: MetricKind::Cos,
            quantization: ScalarKind::F32,
            connectivity: 0,
            expansion_add: 0,
            expansion_search: 0,
            multi: false,
        };
        let index = Index::new(&options).map_err(|e| anyhow::anyhow!("creating index: {}", e))?;

        // Reserve an initial capacity to avoid frequent reallocations.
        index
            .reserve(1024)
            .map_err(|e| anyhow::anyhow!("reserving index capacity: {}", e))?;

        let engine = Engine {
            embedder,
            index,
            id_map: HashMap::new(),
            reverse_map: HashMap::new(),
            next_key: 0,
            dimension: EMBEDDING_DIM,
            index_path,
        };

        // Attempt to reload a previously persisted index.  Failure is
        // non-fatal: we log a warning and proceed with an empty index.
        if std::path::Path::new(&engine.index_path).exists() {
            // Load the raw index; the id maps are not persisted on disk, so
            // we start with empty maps.  The caller is responsible for
            // repopulating the maps if they need round-trip ID→key lookups.
            if let Err(e) = engine.index.load(&engine.index_path) {
                eprintln!(
                    "vecstore warning: failed to load persisted index from {}: {}",
                    engine.index_path, e
                );
            }
        }

        Ok(engine)
    }

    #[cfg(test)]
    fn new_for_test(data_dir: &str) -> Result<Self, anyhow::Error> {
        Self::new_with_embedder(data_dir, Embedder::Test(TestEmbedder))
    }

    /// Embed a single piece of text into a 384-dimensional vector.
    pub fn embed(&self, text: &str) -> Result<Vec<f32>, anyhow::Error> {
        self.embedder.embed(text)
    }

    /// Add a pre-computed vector to the index under the given string `id`.
    ///
    /// If the `id` already exists in the index the old entry is removed first
    /// so the result is an upsert.
    pub fn add(&mut self, id: &str, vector: &[f32]) -> Result<(), anyhow::Error> {
        if vector.len() != self.dimension {
            return Err(anyhow::anyhow!(
                "vector dimension mismatch: expected {}, got {}",
                self.dimension,
                vector.len()
            ));
        }

        // Upsert: remove the old numeric key if this string ID is already known.
        if let Some(&old_key) = self.id_map.get(id) {
            let _ = self.index.remove(old_key);
            self.reverse_map.remove(&old_key);
        }

        let key = self.next_key;
        self.next_key += 1;

        // Grow the index capacity if needed.
        let current_cap = self.index.capacity();
        if self.index.size() + 1 > current_cap {
            self.index
                .reserve(current_cap * 2)
                .map_err(|e| anyhow::anyhow!("growing index capacity: {}", e))?;
        }

        self.index
            .add(key, vector)
            .map_err(|e| anyhow::anyhow!("adding vector to index: {}", e))?;

        self.id_map.insert(id.to_string(), key);
        self.reverse_map.insert(key, id.to_string());

        Ok(())
    }

    /// Search for the `k` nearest neighbours of `query`.
    ///
    /// Returns a list of `(id, distance)` pairs sorted by ascending distance.
    /// The distance is in [0, 2] for cosine metric (0 = identical).
    pub fn search(&self, query: &[f32], k: usize) -> Result<Vec<(String, f32)>, anyhow::Error> {
        if query.len() != self.dimension {
            return Err(anyhow::anyhow!(
                "query dimension mismatch: expected {}, got {}",
                self.dimension,
                query.len()
            ));
        }

        if self.index.size() == 0 {
            return Ok(Vec::new());
        }

        // usearch returns at most index.size() results; cap k accordingly.
        let effective_k = k.min(self.index.size());

        let matches = self
            .index
            .search(query, effective_k)
            .map_err(|e| anyhow::anyhow!("searching index: {}", e))?;

        let results = matches
            .keys
            .iter()
            .zip(matches.distances.iter())
            .map(|(&key, &dist)| {
                // After a reload the id maps are empty; fall back to the raw
                // numeric key as a string so the caller still receives a result.
                let id = self
                    .reverse_map
                    .get(&key)
                    .cloned()
                    .unwrap_or_else(|| key.to_string());
                (id, dist)
            })
            .collect();

        Ok(results)
    }

    /// Delete the vector associated with `id` from the index.
    ///
    /// Returns an error if the `id` is not found.
    pub fn delete(&mut self, id: &str) -> Result<(), anyhow::Error> {
        let key = self
            .id_map
            .remove(id)
            .ok_or_else(|| anyhow::anyhow!("id not found: {}", id))?;

        self.reverse_map.remove(&key);

        self.index
            .remove(key)
            .map_err(|e| anyhow::anyhow!("removing vector from index: {}", e))?;

        Ok(())
    }

    /// Persist the index to disk in the data directory supplied at construction.
    pub fn save(&self) -> Result<(), anyhow::Error> {
        self.index
            .save(&self.index_path)
            .map_err(|e| anyhow::anyhow!("saving index: {}", e))
    }

    /// Return the embedding dimension (384 for AllMiniLM-L6-V2).
    pub fn dimension(&self) -> usize {
        self.dimension
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn make_engine(dir: &str) -> Engine {
        Engine::new_for_test(dir).expect("failed to create test engine")
    }

    fn online_tests_enabled() -> bool {
        matches!(std::env::var("KAIROS_ONLINE_TESTS").as_deref(), Ok("1"))
    }

    fn make_real_engine(dir: &str) -> Engine {
        Engine::new(dir).expect("failed to create real engine")
    }

    #[test]
    fn test_online_engine_creation() {
        if !online_tests_enabled() {
            return;
        }
        let dir = tempfile::tempdir().unwrap();
        let engine = make_real_engine(dir.path().to_str().unwrap());
        assert_eq!(engine.dimension(), EMBEDDING_DIM);
    }

    #[test]
    fn test_online_embed_produces_384_dim() {
        if !online_tests_enabled() {
            return;
        }
        let dir = tempfile::tempdir().unwrap();
        let engine = make_real_engine(dir.path().to_str().unwrap());
        let vec = engine.embed("hello world").unwrap();
        assert_eq!(vec.len(), 384);
    }

    #[test]
    fn test_online_embed_non_zero() {
        if !online_tests_enabled() {
            return;
        }
        let dir = tempfile::tempdir().unwrap();
        let engine = make_real_engine(dir.path().to_str().unwrap());
        let vec = engine.embed("Rust is great").unwrap();
        let has_nonzero = vec.iter().any(|&v| v != 0.0);
        assert!(has_nonzero, "embedding should have non-zero components");
    }

    #[test]
    fn test_add_and_search_round_trip() {
        let dir = tempfile::tempdir().unwrap();
        let mut engine = make_engine(dir.path().to_str().unwrap());

        let vec = engine.embed("persistent memory").unwrap();
        engine.add("doc1", &vec).unwrap();

        let results = engine.search(&vec, 1).unwrap();
        assert_eq!(results.len(), 1);
        assert_eq!(results[0].0, "doc1");
        // cosine distance of a vector with itself should be ~0
        assert!(results[0].1 < 0.01);
    }

    #[test]
    fn test_search_k_greater_than_count() {
        let dir = tempfile::tempdir().unwrap();
        let mut engine = make_engine(dir.path().to_str().unwrap());

        let vec = engine.embed("only one document").unwrap();
        engine.add("sole", &vec).unwrap();

        // Ask for more results than there are documents — should not panic.
        let results = engine.search(&vec, 10).unwrap();
        assert_eq!(results.len(), 1);
    }

    #[test]
    fn test_delete() {
        let dir = tempfile::tempdir().unwrap();
        let mut engine = make_engine(dir.path().to_str().unwrap());

        let v1 = engine.embed("document alpha").unwrap();
        let v2 = engine.embed("document beta").unwrap();
        engine.add("alpha", &v1).unwrap();
        engine.add("beta", &v2).unwrap();

        engine.delete("alpha").unwrap();

        // Search for alpha's vector — it should no longer appear.
        let results = engine.search(&v1, 5).unwrap();
        let ids: Vec<&str> = results.iter().map(|(id, _)| id.as_str()).collect();
        assert!(
            !ids.contains(&"alpha"),
            "deleted id should not appear in results"
        );
    }

    #[test]
    fn test_similar_texts_high_similarity() {
        let dir = tempfile::tempdir().unwrap();
        let mut engine = make_engine(dir.path().to_str().unwrap());

        let v_cat = engine.embed("the cat sat on the mat").unwrap();
        let v_dog = engine.embed("a dog lay on the rug").unwrap();
        let v_rust = engine.embed("Rust ownership and borrowing").unwrap();

        engine.add("cat", &v_cat).unwrap();
        engine.add("dog", &v_dog).unwrap();
        engine.add("rust", &v_rust).unwrap();

        // Query with text similar to "cat" sentence
        let query = engine.embed("the cat rested on the mat").unwrap();
        let results = engine.search(&query, 3).unwrap();

        // The most similar result should be "cat" or "dog" (both animal sentences),
        // and definitely NOT rust which is completely different.
        let top_id = &results[0].0;
        assert!(
            top_id == "cat" || top_id == "dog",
            "expected animal text to be most similar, got {}",
            top_id
        );
    }

    #[test]
    fn test_save_and_reload() {
        let dir = tempfile::tempdir().unwrap();
        let dir_path = dir.path().to_str().unwrap().to_string();

        let original_vec;
        {
            let mut engine = make_engine(&dir_path);
            original_vec = engine.embed("save and reload test").unwrap();
            engine.add("saved_doc", &original_vec).unwrap();
            engine.save().unwrap();
        }

        // The index file must exist on disk.
        let index_file = format!("{}/vecstore.usearch", dir_path);
        assert!(
            std::path::Path::new(&index_file).exists(),
            "index file should exist after save"
        );

        // Re-create the engine — it should load the persisted index and be
        // queryable via the public API.  (The id maps are not persisted, so
        // we can only verify that the ANN index is populated and returns
        // results, not that the original string id is recovered.)
        let engine2 = make_engine(&dir_path);
        assert_eq!(engine2.dimension(), EMBEDDING_DIM);
        let results = engine2.search(&original_vec, 1).unwrap();
        assert_eq!(
            results.len(),
            1,
            "reloaded index should return at least one result"
        );
    }
}
