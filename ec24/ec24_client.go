package main

import (
	"bufio"
	"context"
	"flag"
	"reflect"
	"sync"

	"fmt"
	//"encoding/binary"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	pb "example.com/query"
	"example.com/util"
	"google.golang.org/grpc"
)

const (
	//address = "localhost:50051"
	leftAddress  = "localhost:50051"
	rightAddress = "localhost:50052"
	//address      = "localhost:50051"
	//eps          = 0.5
	//delta        = 1e-6
	FailureProbLog2 = 40
)

var DBSize uint64
var DBSeed uint64
var ChunkSize uint64
var SetSize uint64
var SBSize uint64
var SBSizeLog2 uint64
var SBNum uint64
var threadNum uint64
var serverAddr string
var LogFile *os.File
var str string
var ignoreOffline bool

func runSingleQuery(client pb.QueryServiceClient, DBSize uint64, DBSeed uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(time.Millisecond*500000000000))
	defer cancel()

	log.Printf("Running Network Test")

	rng := rand.New(rand.NewSource(103))
	Q := 1000

	start := time.Now()
	for i := 0; i < Q; i++ {
		//bucket, err := client.SingleQuery(ctx, &pb.CuckooBucketQuery{QueryNum: 1, BucketId: [uint64(i % 10)]})
		j := rng.Uint64() % DBSize
		res, err := client.PlaintextQuery(ctx, &pb.PlaintextQueryMsg{Index: j})

		if len(res.Val) != util.DBEntryLength {
			log.Fatalf("the return value length is %v. Querying for %v", len(res.Val), j)
		}

		resEntry := util.DBEntryFromSlice(res.Val)

		if err != nil {
			log.Fatalf("failed to query %v", err)
		}

		correctVal := util.GenDBEntry(j, DBSeed)

		if util.EntryIsEqual(&resEntry, &correctVal) == false {
			log.Fatalf("wrong value %v at index %v", res.Val, j)
		}

		/*
			log.Printf("Return Value %v", r.GetValue())

			wait_time := time.Duration(poisson_dist.Rand() * 100) * time.Millisecond;
			log.Printf("Waiting Time %v", wait_time)
			time.Sleep(wait_time)
		*/
	}
	elapsed := time.Since(start)
	log.Printf("Non-Private Time: %v ms", float64(elapsed.Milliseconds())/float64(Q))
}

func runFullSetQuery(client pb.QueryServiceClient, DBSize uint64, DBSeed uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(time.Millisecond*500))
	defer cancel()

	rng := rand.New(rand.NewSource(103))
	for i := 0; i < 100; i++ {
		//bucket, err := client.SingleQuery(ctx, &pb.CuckooBucketQuery{QueryNum: 1, BucketId: [uint64(i % 10)]})
		key := util.RandKey(rng)
		res, err := client.FullSetQuery(ctx, &pb.FullSetQueryMsg{PRFKey: key[:]})
		resEntry := util.DBEntryFromSlice(res.Val)

		parity := util.ZeroEntry()
		set := util.PRSet{Key: key}
		ExpandedSet := set.Expand(SetSize, ChunkSize)
		for _, id := range ExpandedSet {
			//log.Printf("%v ", id)
			if id < DBSize {
				entry := util.GenDBEntry(id, DBSeed)
				util.DBEntryXor(&parity, &entry)
			}
		}

		if err != nil {
			log.Fatalf("failed to query %v", err)
		}

		if util.EntryIsEqual(&resEntry, &parity) == false {
			log.Fatalf("wrong value %v at key %v", res.Val, key)
		}

		if i%10 == 0 {
			log.Printf("Got %v-th answer: ", i)
			/*
				for j := 0; j < 1; j++ {
					log.Printf("%v ", bucket.Bucket[j])
				}
			*/
		}
		/*
			log.Printf("Return Value %v", r.GetValue())

			wait_time := time.Duration(poisson_dist.Rand() * 100) * time.Millisecond;
			log.Printf("Waiting Time %v", wait_time)
			time.Sleep(wait_time)
		*/
	}
}

func runBatchedFullSetQuery(client pb.QueryServiceClient, DBSize uint64, DBSeed uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(time.Millisecond*500))
	defer cancel()

	rng := rand.New(rand.NewSource(103))
	batchedFullSetQuery := make([]*pb.FullSetQueryMsg, 0)
	queryNum := uint64(100)
	for i := uint64(0); i < queryNum; i++ {
		//bucket, err := client.SingleQuery(ctx, &pb.CuckooBucketQuery{QueryNum: 1, BucketId: [uint64(i % 10)]})
		key := util.RandKey(rng)
		batchedFullSetQuery = append(batchedFullSetQuery, &pb.FullSetQueryMsg{PRFKey: key[:]})
	}

	res, err := client.BatchedFullSetQuery(ctx, &pb.BatchedFullSetQueryMsg{QueryNum: queryNum, Queries: batchedFullSetQuery})

	if err != nil {
		log.Fatalf("failed to query %v", err)
	}

	for i := uint64(0); i < queryNum; i++ {
		key := batchedFullSetQuery[i].PRFKey
		var prfKey util.PrfKey
		copy(prfKey[:], key)
		parity := util.ZeroEntry()
		set := util.PRSet{Key: prfKey}
		ExpandedSet := set.Expand(SetSize, ChunkSize)
		for _, id := range ExpandedSet {
			//log.Printf("%v ", id)
			if id < DBSize {
				entry := util.GenDBEntry(id, DBSeed)
				util.DBEntryXor(&parity, &entry)
			}
		}

		resEntry := util.DBEntryFromSlice(res.Responses[i].Val)
		if util.EntryIsEqual(&resEntry, &parity) == false {
			log.Fatalf("wrong value %v at key %v", parity, key)
		}

		if i%10 == 0 {
			log.Printf("Got %v-th answer: ", i)
			/*
				for j := 0; j < 1; j++ {
					log.Printf("%v ", bucket.Bucket[j])
				}
			*/
		}
		/*
			log.Printf("Return Value %v", r.GetValue())

			wait_time := time.Duration(poisson_dist.Rand() * 100) * time.Millisecond;
			log.Printf("Waiting Time %v", wait_time)
			time.Sleep(wait_time)
		*/
	}
}

func runPunctSetQuery(client pb.QueryServiceClient, DBSize uint64, DBSeed uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(time.Millisecond*500))
	defer cancel()

	rng := rand.New(rand.NewSource(103))
	for i := 0; i < 100; i++ {
		//bucket, err := client.SingleQuery(ctx, &pb.CuckooBucketQuery{QueryNum: 1, BucketId: [uint64(i % 10)]})
		key := util.RandKey(rng)
		set := util.PRSet{Key: key}
		expandSet := set.Expand(SetSize, ChunkSize)

		punctChunkId := rng.Intn(len(expandSet))
		punctSet := make([]uint64, len(expandSet)-1)
		for i := uint64(0); i < SetSize; i++ {
			if i < uint64(punctChunkId) {
				punctSet[i] = expandSet[i] & (ChunkSize - 1)
			}
			if i > uint64(punctChunkId) {
				punctSet[i-1] = expandSet[i] & (ChunkSize - 1)
			}
		}
		res, err := client.PunctSetQuery(ctx, &pb.PunctSetQueryMsg{PunctSetSize: uint64(len(punctSet)), Indices: punctSet})

		parity := util.ZeroEntry()
		for chunkId, id := range expandSet {
			if i == 0 {
				log.Printf("chunkId %v id %v", chunkId, id)
			}
			if chunkId != punctChunkId {
				if id < DBSize {
					entry := util.GenDBEntry(id, DBSeed)
					util.DBEntryXor(&parity, &entry)
				}
			} else {
				if i == 0 {
					log.Println("punct", chunkId)
				}
			}
		}

		if err != nil {
			log.Fatalf("failed to query %v", err)
		}

		log.Println("parity", parity)

		resEntry := util.DBEntryFromSlice(res.Guesses[punctChunkId*util.DBEntryLength : (punctChunkId+1)*util.DBEntryLength])
		if util.EntryIsEqual(&resEntry, &parity) == false {
			log.Fatalf("wrong value, parity = %v, res = %v, at key %v", parity, resEntry, key)
		}

		if i%10 == 0 {
			log.Printf("Got %v-th answer: ", i)
			/*
				for j := 0; j < 1; j++ {
					log.Printf("%v ", bucket.Bucket[j])
				}
			*/
		}
		/*
			log.Printf("Return Value %v", r.GetValue())

			wait_time := time.Duration(poisson_dist.Rand() * 100) * time.Millisecond;
			log.Printf("Waiting Time %v", wait_time)
			time.Sleep(wait_time)
		*/
	}
}

func runSetParityQuery(client pb.QueryServiceClient, DBSize uint64, DBSeed uint64) {

	// TODO!!!!!!!!!!!!!!!!!!!!!!!!!

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(time.Millisecond*500))
	defer cancel()

	rng := rand.New(rand.NewSource(103))
	key := util.RandKey(rng)

	for i := 0; i < 5; i++ {
		//bucket, err := client.SingleQuery(ctx, &pb.CuckooBucketQuery{QueryNum: 1, BucketId: [uint64(i % 10)]})
		set := util.PRSetWithShortTag{Tag: uint32(i)}
		expandSet := set.Expand(&key, SetSize, ChunkSize)
		//fmt.Printf("The set: %v\n", expandSet)

		res, err := client.SetParityQuery(ctx, &pb.SetParityQueryMsg{Indices: expandSet})

		parity := util.ZeroEntry()
		for _, id := range expandSet {
			//if i == 0 {
			//	log.Printf("chunkId %v id %v", chunkId, id)
			//	}
			entry := util.GenDBEntry(id, DBSeed)
			util.DBEntryXor(&parity, &entry)
		}

		if err != nil {
			log.Fatalf("failed to query %v", err)
		}

		//log.Println("parity", parity)

		//resEntry := util.DBEntryFromSlice(res.Guesses[punctChunkId*util.DBEntryLength : (punctChunkId+1)*util.DBEntryLength])
		queryRes := util.DBEntryFromSlice(res.Parity[:]) /// NOT DONE
		if !util.EntryIsEqual(&queryRes, &parity) {
			log.Fatalf("wrong value, parity = %v, res = %v, at key %v", parity, queryRes, key)
		}

		if i%10 == 0 {
			log.Printf("Got %v-th answer: ", i)
			/*
				for j := 0; j < 1; j++ {
					log.Printf("%v ", bucket.Bucket[j])
				}
			*/
		}
		/*
			log.Printf("Return Value %v", r.GetValue())

			wait_time := time.Duration(poisson_dist.Rand() * 100) * time.Millisecond;
			log.Printf("Waiting Time %v", wait_time)
			time.Sleep(wait_time)
		*/
	}
}

func runPRSetWithListDecodingQuery(client pb.QueryServiceClient, DBSize uint64, DBSeed uint64) {

	// TODO!!!!!!!!!!!!!!!!!!!!!!!!!

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(time.Millisecond*5000))
	defer cancel()

	rng := rand.New(rand.NewSource(103))

	masterKey := util.RandKey(rng)
	longKey := util.GetLongKey((*util.PrfKey128)(&masterKey))

	for t := 0; t < 10; t++ {
		//bucket, err := client.SingleQuery(ctx, &pb.CuckooBucketQuery{QueryNum: 1, BucketId: [uint64(i % 10)]})
		//subkeys := make([]util.PrfKey, SBNum)
		log.Printf("tag %v", t)
		subkeysBytes := make([]byte, SBNum*util.KeyByteLength)
		expandSet := make([]uint64, SetSize)

		for i := uint64(0); i < SBNum; i++ {
			subkey := util.PRFEvalWithLongKeyAndTag128Output(longKey, uint32(t), i)
			//copy(subkeys[i][:], subkey[:])
			copy(subkeysBytes[i*util.KeyByteLength:(i+1)*util.KeyByteLength], subkey[:])

			for j := uint64(0); j < SBSize; j++ {
				expandSet[i*SBSize+j] = util.PRFEval((*util.PrfKey)(&subkey), j) % ChunkSize + (i * SBSize + j) * ChunkSize
			}
		}

		// randomly pick one random superblock. From 0 to SBNum-1
		pickedSuperBlockId := rng.Uint64() % SBNum
		randKey := util.RandKey(rng)
		copy(subkeysBytes[pickedSuperBlockId*util.KeyByteLength:(pickedSuperBlockId+1)*util.KeyByteLength], randKey[:])
		offsets := make([]uint32, SBSize)
		for j := uint64(0); j < SBSize; j++ {
			offsets[j] = uint32(expandSet[pickedSuperBlockId*SBSize+j] % ChunkSize)
		}


		response, err := client.PRSetWithListDecodingQuery(ctx, &pb.PRSetWithListDecodingMsg{SuperBlockKeys: subkeysBytes, Offsets: offsets})
		if err != nil {
			log.Fatalf("failed to query %v", err)
		}
		pariritiesFromListDecoding := response.GetParitiesFromListDecoding()// util.DBEntryFromSlice(response.GetPari)
		res := util.DBEntryFromSlice(pariritiesFromListDecoding[pickedSuperBlockId*util.DBEntryLength:(pickedSuperBlockId+1)*util.DBEntryLength])

		parity := util.ZeroEntry()
		for _, id := range expandSet {
			//if i == 0 {
			//	log.Printf("chunkId %v id %v", chunkId, id)
			//	}
			entry := util.GenDBEntry(id, DBSeed)
			util.DBEntryXor(&parity, &entry)
		}

		if err != nil {
			log.Fatalf("failed to query %v", err)
		}

		//log.Println("parity", parity)

		//resEntry := util.DBEntryFromSlice(res.Guesses[punctChunkId*util.DBEntryLength : (punctChunkId+1)*util.DBEntryLength])
		if !util.EntryIsEqual(&res, &parity) {
			log.Fatalf("wrong value, parity = %v, res = %v, at tag %v", parity, res, t)
		} else {
			log.Printf("The answer is %v", res)
		}
		////////////}

		/*
			log.Printf("Return Value %v", r.GetValue())

			wait_time := time.Duration(poisson_dist.Rand() * 100) * time.Millisecond;
			log.Printf("Waiting Time %v", wait_time)
			time.Sleep(wait_time)
		*/
	}
}

func ReadConfigInfo() (uint64, uint64) {
	file, err := os.Open("config.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	// optionally, resize scanner's capacity for lines over 64K, see next example
	line, _, err := reader.ReadLine()
	if err != nil {
		log.Fatal(err)
	}
	split := strings.Split(string(line), " ")
	var DBSize uint64
	var DBSeed uint64

	if DBSize, err = strconv.ParseUint(split[0], 10, 64); err != nil {
		log.Fatal(err)
	}
	if DBSeed, err = strconv.ParseUint(split[1], 10, 64); err != nil {
		log.Fatal(err)
	}

	log.Printf("%v %v", DBSize, DBSeed)

	return uint64(DBSize), uint64(DBSeed)
}

type LocalSet struct {
	tag             uint32 // the tag of the set
	parity          util.DBEntry
	programmedPoint uint64
	isProgrammed    bool
}

/*
func runPIRWithTwoServer(leftClient pb.QueryServiceClient, rightClient pb.QueryServiceClient, DBSize uint64, DBSeed uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(time.Millisecond*10000000))
	defer cancel()
	seed := time.Now().UnixNano()
	//seed := int64(1678640180140578000)
	log.Println("seed", seed)
	rng := rand.New(rand.NewSource(seed))
	localSetNum := uint64(math.Sqrt(float64(DBSize)) * math.Log(float64(DBSize)))
	totalQueryNum := uint64(math.Sqrt(float64(DBSize))) * 2
	//totalQueryNum := uint64(math.Sqrt(float64(DBSize)))
	localSets := make([]LocalSet, 0)
	localCache := make(map[uint64]uint64)

	// Setup Phase:
	// 		The client first samples sqrt(n) * log(n) random sets
	// 		Sends batch query to the left server and store the parity
	// 		The client only needs to send the keys and the left server will expand them.

	log.Println("Setup Phase")
	start := time.Now()

	batchedFullSetQuery := make([]*pb.FullSetQueryMsg, 0)
	for i := uint64(0); i < localSetNum; i++ {
		key := rng.Uint64()
		batchedFullSetQuery = append(batchedFullSetQuery, &pb.FullSetQueryMsg{PRFKey: key})
	}

	res, err := leftClient.BatchedFullSetQuery(ctx, &pb.BatchedFullSetQueryMsg{QueryNum: localSetNum, Queries: batchedFullSetQuery})

	if err != nil {
		log.Fatalf("failed to batch query full set %v", err)
	}

	for i := uint64(0); i < localSetNum; i++ {
		key := batchedFullSetQuery[i].PRFKey
		localSets = append(localSets, LocalSet{key: key, parity: res.Responses[i].Val, isProgrammed: false, programmedPoint: 0})
	}

	elapsed := time.Since(start)
	log.Printf("Finish Setup Phase, store %v local sets", localSetNum)
	log.Printf("Setup Phase took %v ms, amortized time %v ms per query", elapsed.Milliseconds(), elapsed.Milliseconds()/int64(totalQueryNum))
	log.Printf("Local Storage Size %v MB", localSetNum*(8*3+1)/1024/1024)

	// Online Query Phase:
	start = time.Now()

	for q := uint64(0); q < totalQueryNum; q++ {
		// just do random query for now
		x := rng.Uint64() % DBSize
		//log.Printf("Query %v-th point %v", q, x)

		// make sure x is not in the local cache
		for true {
			if _, ok := localCache[x]; ok == false {
				break
			}
			x = rng.Uint64() % DBSize
		}

		// 		1. Query x: the client first finds a local set that contains x
		// 		2. The client expands the set and punctures the set at x
		// 		3. The client sends the punctured set to the right server and gets sqrt(n)-1 guesses
		//      4. The client picks the correct guesses and xors with the local parity to get DB[x]

		hitSetId := uint64(999999999)

		for i := uint64(0); i < localSetNum; i++ {
			set := util.PRSet{Key: localSets[i].key}
			if localSets[i].isProgrammed && (x/ChunkSize) == (localSets[i].programmedPoint/ChunkSize) {
				if x == localSets[i].programmedPoint {
					log.Fatalf("should not happen x = %v, programmedPoint = %v", x, localSets[i].programmedPoint)
					hitSetId = i
					break
				}
			} else {
				if set.MembTest(x, SetSize, ChunkSize) {
					hitSetId = i
					break
				}
			}
		}

		if hitSetId == 999999999 {
			log.Fatalf("No hit set found for %v in %v-th query", x, q)
		}

		//log.Println("Hit set id", hitSetId)

		// expand the set
		set := util.PRSet{Key: localSets[hitSetId].key}
		expandedSet := set.Expand(SetSize, ChunkSize)
		// manually program the set
		if localSets[hitSetId].isProgrammed {
			//log.Println("Programmed set hit")
			//log.Println("Before programming")
			//for i := uint64(0); i < SetSize; i++ {
			//	log.Printf("%v ", expandedSet[i])
			//	}

			chunkId := localSets[hitSetId].programmedPoint / ChunkSize
			expandedSet[chunkId] = localSets[hitSetId].programmedPoint

		}
		// puncture the set
		punctChunkId := x / ChunkSize
		punctSet := make([]uint64, SetSize-1)
		for i := uint64(0); i < SetSize; i++ {
			if i < punctChunkId {
				// need to mod ChunkSize to make sure the index is in the range of [0, ChunkSize)
				punctSet[i] = expandedSet[i] & (ChunkSize - 1)
			}
			if i > punctChunkId {
				punctSet[i-1] = expandedSet[i] & (ChunkSize - 1)
			}
		}

		// send the punctured set to the right server
		res, err := rightClient.PunctSetQuery(ctx, &pb.PunctSetQueryMsg{PunctSetSize: uint64(len(punctSet)), Indices: punctSet})
		if err != nil {
			log.Fatalf("failed to make punct set query to right server %v", err)
		}

		xVal := localSets[hitSetId].parity ^ res.Guesses[punctChunkId]

		// update the local cache
		localCache[x] = xVal

		// verify the correctness of the query
		if xVal != util.DefaultHash(x^DBSeed) {
			log.Fatalf("wrong value %v at index %v", xVal, x)
		}

		//log.Printf("Correct value %v at index %v", xVal, x)

		// Refresh Phase:
		// 		1. The client samples a random set, punctures it at interval(x) and sends it to the left
		// 		2. The client picks the correct guesses and xors it with DB[x] to get the local parity

		newKey := rng.Uint64()
		newSet := util.PRSet{Key: newKey}
		newPunctSet := make([]uint64, SetSize-1)
		newExpandedSet := newSet.Expand(SetSize, ChunkSize)

		for i := uint64(0); i < SetSize; i++ {
			if i < punctChunkId {
				newPunctSet[i] = newExpandedSet[i] & (ChunkSize - 1)
			}
			if i > punctChunkId {
				newPunctSet[i-1] = newExpandedSet[i] & (ChunkSize - 1)
			}
		}

		res, err = leftClient.PunctSetQuery(ctx, &pb.PunctSetQueryMsg{PunctSetSize: uint64(len(newPunctSet)), Indices: newPunctSet})

		if err != nil {
			log.Fatalf("failed to make refresh punct set query to left server %v", err)
		}

		localSets[hitSetId].key = newKey
		localSets[hitSetId].parity = xVal ^ res.Guesses[punctChunkId]
		localSets[hitSetId].isProgrammed = true
		localSets[hitSetId].programmedPoint = x
	}

	log.Printf("Finish Online Phase with %v queries", totalQueryNum)
	elapsed = time.Since(start)
	log.Printf("Online Phase took %v ms, amortized time %v ms", elapsed.Milliseconds(), elapsed.Milliseconds()/int64(totalQueryNum))
}
*/

type LocalReplacementGroup struct {
	consumed uint64
	indices  []uint64
	value    []util.DBEntry
}

func primaryNumParam(Q float64, ChunkSize float64, target float64) uint64 {
	k := math.Ceil(math.Log(2)*(target) + math.Log(Q))
	log.Printf("k = %v", k)
	return uint64(k) * uint64(ChunkSize)
}

func FailProbBallIntoBins(ballNum uint64, binNum uint64, binSize uint64) float64 {
	log.Printf("ballNum = %v, binNum = %v, binSize = %v", ballNum, binNum, binSize)
	mean := float64(ballNum) / float64(binNum)
	c := float64(binSize)/mean - 1
	log.Printf("mean = %v, c = %v", mean, c)
	// chernoff exp(-(c^2)/(2+c) * mean)
	t := (mean * (c * c) / (2 + c)) * math.Log(2)
	t -= math.Log2(float64(binNum))
	log.Printf("t = %v", t)
	return t
}

type LocalSetEC24 struct {
	Tag uint32
	SetParity util.DBEntry
	BlockParity util.DBEntry
	ProgPointVal util.DBEntry
}

type LocalBackupSetGroupEC24 struct {
	consumed uint64
	sets     []LocalSetEC24
}

func PRSetOffsetEC24(longKey []uint32, tag uint32, chunkId uint64, chunkSize uint64, SBSize uint64, SBSizeLog2 uint64) uint64 {
	subkey := util.PRFEvalWithLongKeyAndTag128Output(longKey, tag, chunkId >> (SBSizeLog2))
	offset := util.PRFEval((*util.PrfKey)(&subkey), chunkId & (SBSize - 1)) & (chunkSize - 1) // 
	return offset
} // return the offset of the chunkId-th chunk in the set

func runPIRWithOneServerEC24(leftClient pb.QueryServiceClient, DBSize uint64, DBSeed uint64) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(time.Millisecond*100000000000))
	defer cancel()
	seed := time.Now().UnixNano()
	rng := rand.New(rand.NewSource(seed))
	masterKey := util.RandKey(rng)
	longKey := util.GetLongKey((*util.PrfKey128)(&masterKey))
	//seed := int64(1678852332934430000)
	totalQueryNum := uint64(math.Sqrt(float64(DBSize)) * math.Log(float64(DBSize)))
	plannedQueryNum := totalQueryNum 
	log.Printf("totalQueryNum %v", totalQueryNum)

	//localSetNum := 4 * uint64(math.Sqrt(float64(DBSize))*math.Log(float64(DBSize)))
	localSetNum := primaryNumParam(float64(totalQueryNum), float64(ChunkSize), FailureProbLog2+1)
	// if localSetNum is not a mulitple of 4 then we need to add some padding
	localSetNum = (localSetNum + threadNum - 1) / threadNum * threadNum
	//backupSetNumPerGroup := 3 * uint64(math.Log(float64(SetSize)))
	backupSetNumPerGroup := 3 * uint64(float64(totalQueryNum)/float64(SetSize))
	backupSetNumPerGroup = (backupSetNumPerGroup + threadNum - 1) / threadNum * threadNum
	//if FailProbBallIntoBins(totalQueryNum, SetSize, backupSetNumPerGroup) < FailureProbLog2+1 {
	//	log.Fatalf("backupSetNumPerGroup is not enough")
	//	}
	//optConst := util.OptimizedFirstBatchSize(int(threadNum))

	totalBackupSetNum := backupSetNumPerGroup * SetSize

	//totalQueryNum = uint64(math.Sqrt(float64(DBSize))) // for the experiment
	totalQueryNum = 1000

	// Setup Phase:
	// 		The client sends a simple query to the left server to fetch the whole DB
	//		When the client gets i-th chunk, it updates all local sets' parities
	//		It also needs to update the backup sets' parities
	//		It also needs to update the i-th group's punct point parity
	start := time.Now()

	// Initialize local sets and backup sets

	localSets := make([]LocalSetEC24, localSetNum)
	localBackupSets := make([]LocalSetEC24, totalBackupSetNum)
	localCache := make(map[uint64]util.DBEntry)
	localMissElements := make(map[uint64]util.DBEntry)
	historyHitIndices := make([]uint64, totalQueryNum) // record the history of the hit sets' indices
	historyQueries := make([]uint64, totalQueryNum)    // record the history of the queries
	positiveConstLables := make([]uint64, localSetNum)  // record the positive constant labels
	tagCounter := uint32(0)

	for j := uint64(0); j < localSetNum; j++ {
		// tmp := rng.Uint64()
		localSets[j] = LocalSetEC24{
			Tag:             tagCounter,
			SetParity:       util.ZeroEntry(),
			BlockParity:     util.ZeroEntry(),
			ProgPointVal:    util.ZeroEntry(),
		}
		tagCounter += 1

		positiveConstLables[j] = uint64(999999999) // initialize the positive constant labels to infinity
	}

	LocalBackupSetGroups := make([]LocalBackupSetGroupEC24, SetSize)
	LocalReplacementGroups := make([]LocalReplacementGroup, SetSize)

	for i := uint64(0); i < SetSize; i++ {
		LocalBackupSetGroups[i].consumed = 0
		LocalBackupSetGroups[i].sets = localBackupSets[i*backupSetNumPerGroup : (i+1)*backupSetNumPerGroup]

		LocalReplacementGroups[i].consumed = 0
		LocalReplacementGroups[i].indices = make([]uint64, backupSetNumPerGroup)
		LocalReplacementGroups[i].value = make([]util.DBEntry, backupSetNumPerGroup)
	}

	for j := uint64(0); j < SetSize; j++ {
		for k := uint64(0); k < backupSetNumPerGroup; k++ {
			LocalBackupSetGroups[j].sets[k] = LocalSetEC24 {
				Tag:             tagCounter,
				SetParity:       util.ZeroEntry(),
				BlockParity:     util.ZeroEntry(),
				ProgPointVal:    util.ZeroEntry(),				
			}
			tagCounter += 1
		}
	}

	log.Printf("Start fetching the whole DB")
	log.Printf("Every Local Set Size %v bytes", reflect.TypeOf(LocalSetEC24{}).Size())
	log.Printf("Local Set Num %v, Local Backup Set Num %v", localSetNum, totalBackupSetNum)

	storage := float64(localSetNum*uint64(reflect.TypeOf(LocalSetEC24{}).Size()) + (totalBackupSetNum * uint64(reflect.TypeOf(LocalSetEC24{}).Size())))
	storage = storage + float64(totalBackupSetNum)*(8+util.DBEntrySize) // the replacements
	storage = storage + float64(totalQueryNum * (8 + 8) + localSetNum * 8) // the size of the history queries, hit indices and the positive constant labels
	log.Printf("Local Storage Size %v MB", storage/1024/1024)


	perQueryCommCost := float64(SBNum) * 16.0 + float64(SBSize) * 4.0 // upload cost
	perQueryCommCost += float64(SBNum) * float64(util.DBEntrySize) * 2.0 // download cost
	log.Printf("Per query communication cost （Online) %v kb", perQueryCommCost/1024)

	if !ignoreOffline {
		// temporal storage
		tempLongKey := make([][]uint32, localSetNum)
		tempLongKeyBackup := make([][]uint32, totalBackupSetNum)

		fechFullDBMsg := &pb.FetchFullDBMsg{Dummy: 1}
		stream, err := leftClient.FetchFullDB(ctx, fechFullDBMsg)

		if err != nil {
			log.Fatalf("failed to fetch full DB %v", err)
		}

		for i := uint64(0); i < SetSize; i++ {
			if i % SBSize == 0 {
				for j := uint64(0); j < localSetNum; j++ {
					tempSubKey := util.PRFEvalWithLongKeyAndTag128Output(longKey, localSets[j].Tag, i >> SBSizeLog2)
					tempLongKey[j] = util.GetLongKey((*util.PrfKey128)(&tempSubKey))
				}
				for j := uint64(0); j < totalBackupSetNum; j++ {
					tempSubKey := util.PRFEvalWithLongKeyAndTag128Output(longKey, localBackupSets[j].Tag, i >> SBSizeLog2)
					tempLongKeyBackup[j] = util.GetLongKey((*util.PrfKey128)(&tempSubKey))
				}
			}

			chunk, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Fatalf("failed to receive chunk %v", err)
			}
			if i%1000 == 0 {
				log.Printf("received chunk %v", i)
			}
			//log.Printf("received chunk %v", i)
			hitMap := make([]bool, ChunkSize)

			// use multiple threads to parallelize the computation for the chunk

			var wg sync.WaitGroup
			wg.Add(int(threadNum))

			perTheadSetNum := (localSetNum+threadNum-1)/threadNum + 1 // make sure all sets are covered
			perThreadBackupNum := (totalBackupSetNum+threadNum-1)/threadNum + 1

			//start := time.Now()

			for tid := uint64(0); tid < threadNum; tid++ {
				startIndex := uint64(tid) * uint64(perTheadSetNum)
				endIndex := startIndex + uint64(perTheadSetNum)
				if endIndex > localSetNum {
					endIndex = localSetNum
				}

				startIndexBackup := uint64(tid) * uint64(perThreadBackupNum)
				endIndexBackup := startIndexBackup + uint64(perThreadBackupNum)
				if endIndexBackup > totalBackupSetNum {
					endIndexBackup = totalBackupSetNum
				}

				go func(start, end, start1, end1 uint64) {
					defer wg.Done()
					for j := uint64(start); j < uint64(end); j++ {
						//offset := PRSetOffsetEC24(longKey, localSets[j].Tag, i, ChunkSize, SBSize, SBSizeLog2)
						offset := util.PRFEvalWithLongKey(tempLongKey[j], i & (SBSize - 1)) & (ChunkSize - 1)
						hitMap[offset] = true
						//localSets[j].parity ^= chunk.Chunk[offset]
						util.DBEntryXorFromRaw(&localSets[j].SetParity, chunk.Chunk[offset*util.DBEntryLength:(offset+1)*util.DBEntryLength])
					}
					for j := uint64(start1); j < uint64(end1); j++ {
						offset := util.PRFEvalWithLongKey(tempLongKeyBackup[j], i & (SBSize - 1)) & (ChunkSize - 1)
						//offset := PRSetOffsetEC24(longKey, localBackupSets[j].Tag, i, ChunkSize, SBSize, SBSizeLog2)
						util.DBEntryXorFromRaw(&localBackupSets[j].SetParity, chunk.Chunk[offset*util.DBEntryLength:(offset+1)*util.DBEntryLength])
					}
				}(startIndex, endIndex, startIndexBackup, endIndexBackup)
			}

			wg.Wait()

			//fullSetTime := time.Since(start)
			//log.Printf("fullsetTime %v ms", fullSetTime.Milliseconds())

			for j := uint64(0); j < ChunkSize; j++ {
				if hitMap[j] == false {
					entry := util.DBEntryFromSlice(chunk.Chunk[j*util.DBEntryLength : (j+1)*util.DBEntryLength])
					localMissElements[j+i*ChunkSize] = entry
				}
			}


			//start = time.Now()
			// for the i-th group of backups, set the ProgPointVal to the i-th chunk
			for k := uint64(0); k < backupSetNumPerGroup; k++ {
				//key := &LocalBackupSetGroups[i].sets[k].key
				offset := PRSetOffsetEC24(longKey, LocalBackupSetGroups[i].sets[k].Tag, i, ChunkSize, SBSize, SBSizeLog2)
				util.DBEntryXorFromRaw(&LocalBackupSetGroups[i].sets[k].ProgPointVal, chunk.Chunk[offset*util.DBEntryLength:(offset+1)*util.DBEntryLength])
			}

			// for the groups in the same superblock, update their block parities
			SBId := i / SBSize
			for j := SBId * SBSize; j < (SBId+1)*SBSize; j++ {
				for k := uint64(0); k < backupSetNumPerGroup; k++ {
					offset := PRSetOffsetEC24(longKey, LocalBackupSetGroups[j].sets[k].Tag, i, ChunkSize, SBSize, SBSizeLog2)
					util.DBEntryXorFromRaw(&LocalBackupSetGroups[j].sets[k].BlockParity, chunk.Chunk[offset*util.DBEntryLength:(offset+1)*util.DBEntryLength])
				}
			}
			//blockProcTime := time.Since(start)
			//log.Printf("blockProcTime %v ms", blockProcTime.Milliseconds())

			// store the replacement
			for k := uint64(0); k < backupSetNumPerGroup; k++ {
				// generate a random offset between 0 and ChunkSize - 1
				offset := rng.Uint64() & (ChunkSize - 1)
				LocalReplacementGroups[i].indices[k] = offset + i*ChunkSize
				LocalReplacementGroups[i].value[k] = util.DBEntryFromSlice(chunk.Chunk[offset*util.DBEntryLength : (offset+1)*util.DBEntryLength])
			}
		}
	}

	elapsed := time.Since(start)
	offlineElapsed := elapsed
	offlineCommCost := float64(DBSize) * float64(reflect.TypeOf(util.DBEntry{}).Size())

	log.Printf("Finish Setup Phase, store %v local sets, %v backup sets/replacement pairs", localSetNum, SetSize*backupSetNumPerGroup)
	//log.Printf("Local Storage Size %v MB", float64(localSetNum*uint64(reflect.TypeOf(LocalSet{}).Size())+(totalBackupSetNum*uint64(reflect.TypeOf(LocalBackupSet{}).Size())))/1024/1024)
	//log.Printf("Local Storage Size %v MB", localStorageSize/1024/1024)
	setupTime := float64(offlineElapsed.Milliseconds())
	log.Printf("-----offline stats--------")
	log.Printf("Local Storage Size %v MB", storage/1024/1024)
	log.Printf("Setup Phase took %v ms, amortized time %v ms per query", setupTime, setupTime/float64(plannedQueryNum))
	//log.Printf("Setup Phase took %v ms, amortized time %v ms per query", elapsed.Milliseconds(), float64(elapsed.Milliseconds())/float64(plannedQueryNum))
	log.Printf("Setup Phase Comm Cost %v MB, amortized cost %v KB per query", float64(offlineCommCost)/1024/1024, float64(offlineCommCost)/1024/float64(plannedQueryNum))
	log.Printf("Num of local miss elements %v", len(localMissElements))
	log.Printf("-------------------")

	// Online Query Phase:
	totalNetworkLatency := uint64(0)
	totalServerComputeTime := uint64(0)
	totalFindHintTime := uint64(0)
	totalCase1 := uint64(0)
	totalCase2 := uint64(0)

	start = time.Now()

	//for q := uint64(0); q < 1; q++ {
	for q := uint64(0); q < totalQueryNum; q++ {
		if q%100 == 0 {
			log.Printf("Making %v-th query", q)
		}
		// just do random query for now
		x := rng.Uint64() % DBSize

		// make sure x is not in the local cache
		for true {
			if _, ok := localCache[x]; ok == false {
				break
			}
			x = rng.Uint64() % DBSize
		}
		historyQueries[q] = x

		// 		1. Query x: the client first finds a local set that contains x
		// There are 3 subcases: // a) this set does not have a positive constraint.
		hitSetId := uint64(999999999)

		findHintStart := time.Now()

		queryOffset := x % ChunkSize
		chunkId := x / ChunkSize

		for i := uint64(0); i < localSetNum; i++ {
			if PRSetOffsetEC24(longKey, localSets[i].Tag, chunkId, ChunkSize, SBSize, SBSizeLog2) == queryOffset {
				if positiveConstLables[i] != uint64(999999999) && chunkId == (uint64(positiveConstLables[i])/ChunkSize) {
					continue
				}
				hitSetId = i
				break
			}
		}

		/*
		// no need to verify

		if q == 0 {
			parity := util.ZeroEntry()
			for i := uint64(0); i < SetSize; i++ {
				index := PRSetOffsetEC24(longKey, localSets[hitSetId].Tag, i, ChunkSize, SBSize, SBSizeLog2) + i * ChunkSize
				fmt.Printf("%v ", index)
				originalVal := util.GenDBEntry(index, DBSeed)
				util.DBEntryXor(&parity, &originalVal)
			}
			fmt.Printf("\n")
			if util.EntryIsEqual(&parity, &localSets[hitSetId].SetParity) == false {
				log.Fatalf("preprocessed parity is not correct")
			} else {
				log.Printf("preprocessed parity is correct %v", parity)
			}
		}

		*/

		findHintElapsed := time.Since(findHintStart)
		totalFindHintTime += uint64(findHintElapsed.Nanoseconds())
		// if still no hit set found, then fail
		if hitSetId == 999999999 {
			log.Fatalf("No hit set found for %v in %v-th query", x, q)
		}

		historyHitIndices[q] = hitSetId
		y := positiveConstLables[hitSetId]

		var xVal util.DBEntry
		// determine which case
		caseId := 0
		if y != uint64(999999999) {
			if y / (SBSize * ChunkSize) == x / (SBSize * ChunkSize) {
				// positive const, and it's in the same superblock
				totalCase1 += 1
				caseId = 1
			} else {
				// positive const, but it's not in the same superblock
				totalCase2 += 1
				caseId = 2
			}
		}

		// now we expand the set. this is the common case -- case 0
		subkeyBytes := make([]byte, SBNum*util.KeyByteLength)
		offsets := make([]uint64, SBSize)
		SBofX := x / (SBSize * ChunkSize)
		for i := uint64(0); i < SBNum; i++ {
			subkey := util.PRFEvalWithLongKeyAndTag128Output(longKey, localSets[hitSetId].Tag, i)
			copy(subkeyBytes[i*util.KeyByteLength:(i+1)*util.KeyByteLength], subkey[:])
		}

		// the offsets in x's superblock
		for j := uint64(0); j < SBSize; j++ {
			offsets[j] = PRSetOffsetEC24(longKey, localSets[hitSetId].Tag, SBofX * SBSize + j, ChunkSize, SBSize, SBSizeLog2)
		}

		// find a replacement from x's replacement group
		nxtAvailable := LocalReplacementGroups[chunkId].consumed
		repOffset := rng.Uint64()%ChunkSize // a dummy random offset
		repVal := util.ZeroEntry()
		if nxtAvailable == backupSetNumPerGroup {
			log.Printf("No replacement available for %v-th query", q)
		} else {
			// consume one replacement
			repOffset = LocalReplacementGroups[chunkId].indices[nxtAvailable] % ChunkSize
			repVal = LocalReplacementGroups[chunkId].value[nxtAvailable]
			LocalReplacementGroups[chunkId].consumed++
		}
		offsets[chunkId % SBSize] = repOffset
		//now needs to replace the superblock(x)-th key with a random key
		randKey := util.RandKey(rng)
		copy(subkeyBytes[(SBofX)*util.KeyByteLength:(SBofX+1)*util.KeyByteLength], randKey[:])


		// now for case 1, we need to further edit the offset in chunk(y) 
		if caseId == 1 {
			offsets[(y / ChunkSize) % SBSize] = y % ChunkSize
		}

		// now for case 2, we need to do rejection sampling. 
		if caseId == 2 {
			// first we need to build the constraints. 
			// we know the positive constraint is y. 
			// we only need to know the negative constraint that is in the same superblock as y.

			negConst := make([]uint64, 0)

			for i := int(q) - 1; i >= 0; i-- {
				if historyHitIndices[i] == hitSetId {
					break // we already found all the negative constraints that are older than the positive constraint
				}
				if historyHitIndices[i] > hitSetId {
					// if the hit index is larger than the current hit index, it's a negative const
					// we only need to record the negative const that is in the same superblock as y
					if historyQueries[i] / (SBSize * ChunkSize) == y / (SBSize * ChunkSize) {
						negConst = append(negConst, historyQueries[i])
					}
				}
			}

			//log.Printf("neg const number %v", len(negConst))
			//log.Printf("neg const %v", negConst)

			// now we need to do rejection sampling 
			randKey := util.RandKey(rng)
			yOffset := y % ChunkSize
			yChunkId := y / ChunkSize
			yRelativeChunkId := yChunkId % SBSize
			for true {
				// first check the positive const
				newOffset := util.PRFEval(&randKey, yRelativeChunkId) & (ChunkSize - 1)
				if newOffset != yOffset {
					randKey = util.RandKey(rng)
					continue
				}
				//log.Printf("positive const ok")
				// now check the negative const
				ok := true
				for i := 0; i < len(negConst); i++ {
					tmpOffset := util.PRFEval(&randKey, (negConst[i] / ChunkSize) % SBSize) % ChunkSize
					if tmpOffset == negConst[i] % ChunkSize {
						ok = false
						break
					}
				}
				if ok == true {
					break
				} else {
					randKey = util.RandKey(rng)
				}
			}

			// now copy this key to the subkeyBytes
			// replace the superblock(y)-th key
			copy(subkeyBytes[(yChunkId / SBSize)*util.KeyByteLength:(yChunkId / SBSize + 1)*util.KeyByteLength], randKey[:])
		}
		offsetsuint32 := make([]uint32, len(offsets))
		for i := 0; i < len(offsets); i++ {
			offsetsuint32[i] = uint32(offsets[i])
		}

		// send the edited set to the server
		networkStart := time.Now()
		res, err := leftClient.PRSetWithListDecodingQuery(ctx, &pb.PRSetWithListDecodingMsg{SuperBlockKeys: subkeyBytes, Offsets: offsetsuint32})
		networkSince := time.Since(networkStart)
		if err != nil {
			log.Fatalf("failed to make punct set query to server %v", err)
		}

		remoteTotalTime := networkSince.Nanoseconds()
		networkLatency := uint64(remoteTotalTime) - res.ServerComputeTime

		totalNetworkLatency += networkLatency
		totalServerComputeTime += res.ServerComputeTime

		// this is the common case 0
		xVal = localSets[hitSetId].SetParity                              // the parity of the hit set
		util.DBEntryXorFromRaw(&xVal, res.ParitiesFromListDecoding[SBofX*util.DBEntryLength:(SBofX+1)*util.DBEntryLength]) // xor the parity of the edited set
		util.DBEntryXor(&xVal, &repVal)                                // xor the replacement value

		if caseId == 1 {
			// in this case, we just need to xor the parity of y and the original point in chunk(y)
			util.DBEntryXor(&xVal, &localSets[hitSetId].ProgPointVal)
		}

		if caseId == 2 {
			// first xor the alpha_superblock(y)
			util.DBEntryXorFromRaw(&xVal, res.ParitiesFromKeys[y / (ChunkSize * SBSize) * util.DBEntryLength:(y / (ChunkSize * SBSize) + 1) * util.DBEntryLength])
			// second, xor the original block's parity
			util.DBEntryXor(&xVal, &localSets[hitSetId].BlockParity)
			//log.Printf("removing the block parity")
		}

		// update the local cache
		localCache[x] = xVal

		// verify the correctness of the query
		entry := util.GenDBEntry(DBSeed, x)
		// if ignoreOffline == true, the client will not verify the correctness of the query
		if ignoreOffline == false && util.EntryIsEqual(&xVal, &entry) == false {
			log.Fatalf("wrong value %v at index %v at query %v", xVal, x, q)
		} else {
			if q == 0 {
				log.Printf("Correct value %v at index %v", xVal, x)
			}
			//log.Printf("Correct value %v at index %v", xVal, x)
		}

		//log.Printf("Correct value %v at index %v", xVal, x)

		// Regresh Phase:
		// The client picks one set from the x's backup group
		// adds the x to the set and adds the set to the local set list

		if LocalBackupSetGroups[chunkId].consumed == backupSetNumPerGroup {
			log.Printf("consumed %v sets", LocalBackupSetGroups[chunkId].consumed)
			log.Printf("backupSetNumPerGroup %v", backupSetNumPerGroup)
			log.Fatalf("No backup set available for %v-th query", q)
		}

		consumed := LocalBackupSetGroups[chunkId].consumed
		localSets[hitSetId].Tag = LocalBackupSetGroups[chunkId].sets[consumed].Tag
		localSets[hitSetId].SetParity = LocalBackupSetGroups[chunkId].sets[consumed].SetParity
		localSets[hitSetId].BlockParity = LocalBackupSetGroups[chunkId].sets[consumed].BlockParity
		localSets[hitSetId].ProgPointVal = LocalBackupSetGroups[chunkId].sets[consumed].ProgPointVal
		// edit the progpointval
		util.DBEntryXor(&localSets[hitSetId].ProgPointVal, &xVal)
		LocalBackupSetGroups[chunkId].consumed++
		// mark the positive const
		positiveConstLables[hitSetId] = x 
	}

	elapsed = time.Since(start)
	perQueryUploadCost := float64(SBNum) * 16.0 + float64(SBSize) * 4.0 // upload cost
	perQueryDownloadCost := float64(SBNum) * float64(util.DBEntrySize) * 2.0 // download cost

	avgNetworkLatency := float64(totalNetworkLatency) / float64(totalQueryNum)
	avgServerComputeTime := float64(totalServerComputeTime) / float64(totalQueryNum)
	avgAmortizedTime := float64(elapsed.Nanoseconds()) / float64(totalQueryNum)
	avgClientComputeTime := avgAmortizedTime - avgNetworkLatency - avgServerComputeTime
	avgFindHintTime := float64(totalFindHintTime) / float64(totalQueryNum)

	log.Printf("Finish Online Phase with %v queries", totalQueryNum)
	log.Printf("Ratio of case 1 %v", float64(totalCase1)/float64(totalQueryNum))
	log.Printf("Ratio of case 2 %v", float64(totalCase2)/float64(totalQueryNum))
	log.Printf("-----offline stats--------")
	log.Printf("Local Storage Size %v MB", storage/1024/1024)
	log.Printf("Setup Phase took %v ms, amortized time %v ms per query", setupTime, setupTime/float64(plannedQueryNum))
	//log.Printf("Setup Phase took %v ms, amortized time %v ms per query", elapsed.Milliseconds(), float64(elapsed.Milliseconds())/float64(plannedQueryNum))
	log.Printf("Setup Phase Comm Cost %v MB, amortized cost %v KB per query", float64(offlineCommCost)/1024/1024, float64(offlineCommCost)/1024/float64(plannedQueryNum))
	log.Printf("-------------------")

	log.Printf("----------------stats --------------------")

	log.Printf("Online Phase took %v ms, amortized time %v ms", elapsed.Milliseconds(), float64(elapsed.Milliseconds())/float64(totalQueryNum))
	log.Printf("Per query upload cost %v kb", perQueryUploadCost/1024)
	log.Printf("Per query download cost %v kb", perQueryDownloadCost/1024)
	log.Printf("End to end storage %v MB", storage/1024/1024)
	log.Printf("End to end amortized time %v ms", float64(offlineElapsed.Milliseconds())/float64(plannedQueryNum)+float64(elapsed.Milliseconds())/float64(totalQueryNum))
	log.Printf("End to end amortized comm cost %v kb", (float64(offlineCommCost)/1024/float64(plannedQueryNum) + (perQueryUploadCost+perQueryDownloadCost)/1024))

	log.Printf("---------------breakdown-------------------------")
	log.Printf("End to end amortized time %v ms", float64(offlineElapsed.Milliseconds())/float64(plannedQueryNum)+float64(elapsed.Milliseconds())/float64(totalQueryNum))
	log.Printf("Average Online Time %v ms", avgAmortizedTime/1000000)
	log.Printf("Average Network Latency %v ms", avgNetworkLatency/1000000)
	log.Printf("Average Server Time %v ms", avgServerComputeTime/1000000)
	log.Printf("Average Client Time %v ms", avgClientComputeTime/1000000)
	log.Printf("Average Find Hint Time %v ms", avgFindHintTime/1000000)
	log.Printf("-------------------------------------------------")

	LogFile.WriteString("--------------------stats--------------------------\n")
	str := fmt.Sprintf("Current Time: %v\n", time.Now().Format("2006-01-02 15:04:05"))
	LogFile.WriteString(str)

	LogFile.WriteString("Name: EC24 n^{1/4} PIR (QuaterPIR)\n")

	str = fmt.Sprintf("DB n: %v\n", DBSize)
	LogFile.WriteString(str)

	str = fmt.Sprintf("DB entry size (Bytes): %v\n", util.DBEntrySize)
	LogFile.WriteString(str)

	str = fmt.Sprintf("DB size (GB): %v\n", float64(DBSize)*float64(util.DBEntrySize)/1024/1024/1024)
	LogFile.WriteString(str)

	str = fmt.Sprintf("Prep. Time (min): %v\n", float64(offlineElapsed.Milliseconds())/1000/60)
	LogFile.WriteString(str)

	str = fmt.Sprintf("Am. Prep. Time (ms): %v\n", float64(offlineElapsed.Milliseconds())/float64(plannedQueryNum))
	LogFile.WriteString(str)

	str = fmt.Sprintf("Prep. Comm. (GB): %v\n", (float64(offlineCommCost)/1024/1024/1024))
	LogFile.WriteString(str)

	str = fmt.Sprintf("Am. Prep. Comm. (KB): %v\n", (float64(offlineCommCost)/1024/float64(plannedQueryNum)))
	LogFile.WriteString(str)

	str = fmt.Sprintf("Avg. Online Time (ms): %v\n", avgAmortizedTime/1000000)
	LogFile.WriteString(str)

	str = fmt.Sprintf("Online Comm. (KB): %v\n", (perQueryUploadCost+perQueryDownloadCost)/1024)
	LogFile.WriteString(str)

	str = fmt.Sprintf("Storage (MB): %v\n", storage/1024/1024)
	LogFile.WriteString(str)

	LogFile.WriteString("---------------------------------------------------\n\n\n")
}


func main() {
	//log.Printf("%v", primaryNumParam(870019, 131072, 40))
	addrPtr := flag.String("ip", "localhost:50051", "port number")
	threadPtr := flag.Int("thread", 1, "number of threads")
	ignoreOfflinePtr := flag.Bool("ignoreOffline", false, "ignore offline phase")
	flag.Parse()

	serverAddr = *addrPtr
	threadNum = uint64(*threadPtr)
	ignoreOffline = *ignoreOfflinePtr
	log.Printf("Server address %v, thread number %v ignoreOffline %v", serverAddr, threadNum, ignoreOffline)

	DBSize, DBSeed = ReadConfigInfo()
	//ChunkSize, SetSize = util.GenParams(DBSize)
	ChunkSize, SetSize, SBSize, SBNum = util.GenParams(DBSize)

	SBSizeLog2 = uint64(math.Log2(float64(SBSize)))
	log.Printf("SBSizeLog2 %v", SBSizeLog2)

	maxMsgSize := 12 * 1024 * 1024

	f, _ := os.OpenFile("output.txt", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	LogFile = f

	log.Printf("DBSize %v, DBSeed %v, ChunkSize %v, SetSize %v, SBSize %v, SBNum %v", DBSize, DBSeed, ChunkSize, SetSize, SBSize, SBNum)

	leftConn, err := grpc.Dial(
		serverAddr,
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMsgSize), grpc.MaxCallSendMsgSize(maxMsgSize)),
	)
	if err != nil {
		log.Fatalf("Failed to connect server %v", leftAddress)
	}
	leftClient := pb.NewQueryServiceClient(leftConn)

	defer leftConn.Close()

	/*
		rightConn, err := grpc.Dial(rightAddress, grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			log.Fatalf("Failed to connect server %v", rightAddress)
		}
		defer rightConn.Close()

		rightClient := pb.NewQueryServiceClient(rightConn)
	*/

	//runHashTableInfoQuery(c)

	//runSingleQuery(leftClient, DBSize, DBSeed)
	//runSetParityQuery(leftClient, DBSize, DBSeed)
	//runFullSetQuery(c, DBSize, DBSeed)
	//runPunctSetQuery(leftClient, DBSize, DBSeed)
	//runBatchedFullSetQuery(c, DBSize, DBSeed)
	//runPIRWithTwoServer(leftClient, rightClient, DBSize, DBSeed)

	//runPRSetWithListDecodingQuery(leftClient, DBSize, DBSeed)
	runPIRWithOneServerEC24(leftClient, DBSize, DBSeed)

	//runPIRWithOneServer(leftClient, DBSize, DBSeed)

	//runSingleQuery(c);
	//runContinuousQuery(c)

}
