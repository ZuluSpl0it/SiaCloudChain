package crypto

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/NebulousLabs/fastrand"
)

// TestTreeBuilder builds a tree and gets the merkle root.
func TestTreeBuilder(t *testing.T) {
	tree := NewTree()
	tree.PushObject("a")
	tree.PushObject("b")
	_ = tree.Root()

	// Correctness is assumed, as it's tested by the merkletree package. This
	// function is really for code coverage.
}

// TestCalculateLeaves probes the CalculateLeaves function.
func TestCalculateLeaves(t *testing.T) {
	tests := []struct {
		size, expSegs uint64
	}{
		{0, 1},
		{63, 1},
		{64, 1},
		{65, 2},
		{127, 2},
		{128, 2},
		{129, 3},
	}

	for i, test := range tests {
		if segs := CalculateLeaves(test.size); segs != test.expSegs {
			t.Errorf("miscalculation for test %v: expected %v, got %v", i, test.expSegs, segs)
		}
	}
}

// TestStorageProof builds a storage proof and checks that it verifies
// correctly.
func TestStorageProof(t *testing.T) {
	// Generate proof data.
	numSegments := uint64(7)
	data := fastrand.Bytes(int(numSegments * SegmentSize))
	rootHash := MerkleRoot(data)

	// Create and verify proofs for all indices.
	for i := uint64(0); i < numSegments; i++ {
		baseSegment, hashSet := MerkleProof(data, i)
		if !VerifySegment(baseSegment, hashSet, numSegments, i, rootHash) {
			t.Fatal("Proof", i, "did not pass verification")
		}
	}

	// Try an incorrect proof.
	baseSegment, hashSet := MerkleProof(data, 3)
	if VerifySegment(baseSegment, hashSet, numSegments, 4, rootHash) {
		t.Error("Verified a bad proof")
	}
}

// TestNonMultipleNumberOfSegmentsStorageProof builds a storage proof that has
// a last leaf of size less than SegmentSize.
func TestNonMultipleLeafSizeStorageProof(t *testing.T) {
	// Generate proof data.
	data := fastrand.Bytes((2 * SegmentSize) + 10)
	rootHash := MerkleRoot(data)

	// Create and verify a proof for the last index.
	baseSegment, hashSet := MerkleProof(data, 2)
	if !VerifySegment(baseSegment, hashSet, 3, 2, rootHash) {
		t.Error("padded segment proof failed")
	}
}

// TestRangeProof tests all possible range proofs for a small tree.
func TestRangeProof(t *testing.T) {
	// Create and verify proofs for all possible ranges.
	for numSegments := 0; numSegments < 34; numSegments++ {
		data := fastrand.Bytes(numSegments * SegmentSize)
		root := MerkleRoot(data)
		for start := 0; start < numSegments; start++ {
			for end := start + 1; end < numSegments+1; end++ {
				proofData := data[start*SegmentSize : end*SegmentSize]
				proof := MerkleRangeProof(data, start, end)
				if !VerifyRangeProof(proofData, proof, start, end, root) {
					t.Errorf("Proof %v-%v did not pass verification", start, end)
				}
			}
		}
	}
}

// TestCachedTree tests the cached tree functions of the package.
func TestCachedTree(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Build a cached tree out of 4 subtrees, each subtree of height 2 (4
	// elements).
	tree1Bytes := fastrand.Bytes(SegmentSize * 4)
	tree2Bytes := fastrand.Bytes(SegmentSize * 4)
	tree3Bytes := fastrand.Bytes(SegmentSize * 4)
	tree4Bytes := fastrand.Bytes(SegmentSize * 4)
	tree1Root := MerkleRoot(tree1Bytes)
	tree2Root := MerkleRoot(tree2Bytes)
	tree3Root := MerkleRoot(tree3Bytes)
	tree4Root := MerkleRoot(tree4Bytes)
	fullRoot := MerkleRoot(append(tree1Bytes, append(tree2Bytes, append(tree3Bytes, tree4Bytes...)...)...))

	// Get a cached proof for index 0.
	base, cachedHashSet := MerkleProof(tree1Bytes, 0)
	if !VerifySegment(base, cachedHashSet, 4, 0, tree1Root) {
		t.Fatal("the proof for the subtree was invalid")
	}
	ct := NewCachedTree(2)
	ct.SetIndex(0)
	ct.Push(tree1Root)
	ct.Push(tree2Root)
	ct.Push(tree3Root)
	ct.Push(tree4Root)
	hashSet := ct.Prove(base, cachedHashSet)
	if !VerifySegment(base, hashSet, 4*4, 0, fullRoot) {
		t.Fatal("cached proof construction appears unsuccessful")
	}
	if ct.Root() != fullRoot {
		t.Fatal("cached Merkle root is not matching the full Merkle root")
	}

	// Get a cached proof for index 6.
	base, cachedHashSet = MerkleProof(tree2Bytes, 2)
	if !VerifySegment(base, cachedHashSet, 4, 2, tree2Root) {
		t.Fatal("the proof for the subtree was invalid")
	}
	ct = NewCachedTree(2)
	ct.SetIndex(6)
	ct.Push(tree1Root)
	ct.Push(tree2Root)
	ct.Push(tree3Root)
	ct.Push(tree4Root)
	hashSet = ct.Prove(base, cachedHashSet)
	if !VerifySegment(base, hashSet, 4*4, 6, fullRoot) {
		t.Fatal("cached proof construction appears unsuccessful")
	}
	if ct.Root() != fullRoot {
		t.Fatal("cached Merkle root is not matching the full Merkle root")
	}
}

// TestMerkleTreeOddDataSize checks that MerkleRoot and MerkleProof still
// function correctly if you provide data which does not have a size evenly
// divisible by SegmentSize.
func TestOddDataSize(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}

	// Create some random data that's not evenly padded.
	for i := 0; i < 25; i++ {
		randFullSegments := fastrand.Intn(65)
		randOverflow := fastrand.Intn(63) + 1
		randProofIndex := fastrand.Intn(randFullSegments + 1)
		data := fastrand.Bytes(SegmentSize*randFullSegments + randOverflow)
		root := MerkleRoot(data)
		base, hashSet := MerkleProof(data, uint64(randProofIndex))
		if !VerifySegment(base, hashSet, uint64(randFullSegments)+1, uint64(randProofIndex), root) {
			t.Error("Padded data proof failed for", randFullSegments, randOverflow, randProofIndex)
		}
	}
}

// TestClone checks that clone makes copy of existed tree
// and the clone has the same content,
// produces same root and have different pointers.
func TestClone(t *testing.T) {
	tree := NewTree()

	for i := 0; i < 15; i++ {
		tree.PushObject(i)
	}

	// Cloned and origin tree are the same at the beginning.
	clonedTree := tree.Clone()
	require.Equal(t, tree, clonedTree)
	require.Equal(t, tree.Root(), clonedTree.Root())

	// Push new items to cloned tree, origin tree stayed the same.
	treeRoot := tree.Root()
	for i := 0; i < 5; i++ {
		clonedTree.PushObject(i)
	}
	require.Equal(t, treeRoot, tree.Root())

	// Cloned tree changed.
	require.NotEqual(t, tree, clonedTree)
	require.NotEqual(t, tree.Root(), clonedTree.Root())

	// Copy only Tree struct value, not using Clone().
	// Push object to one tree, it is changed,
	// then push to the copied tree, both are changed now and became identical.
	sameTree := &MerkleTree{Tree: clonedTree.Tree}
	clonedTree.PushObject(1)
	require.NotEqual(t, clonedTree.Root(), sameTree.Root())
	sameTree.PushObject(2)
	require.Equal(t, clonedTree.Root(), sameTree.Root())
}
