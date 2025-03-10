package main

import (
	"encoding/csv"
	"fmt"

	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/tuneinsight/lattigo/v6/core/rlwe"
	"github.com/tuneinsight/lattigo/v6/multiparty"
	"github.com/tuneinsight/lattigo/v6/ring"
	"github.com/tuneinsight/lattigo/v6/schemes/bgv"
	"github.com/tuneinsight/lattigo/v6/utils/sampling"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func runTimed(f func()) time.Duration {
	start := time.Now()
	f()
	return time.Since(start)
}

func runTimedParty(f func(), N int) time.Duration {
	start := time.Now()
	f()
	return time.Duration(time.Since(start).Nanoseconds() / int64(N))
}

type party struct {
	sk         *rlwe.SecretKey
	rlkEphemSk *rlwe.SecretKey

	ckgShare    multiparty.PublicKeyGenShare
	rkgShareOne multiparty.RelinearizationKeyGenShare
	rkgShareTwo multiparty.RelinearizationKeyGenShare
	pcksShare   multiparty.PublicKeySwitchShare

	input []uint64
}
type multTask struct {
	wg              *sync.WaitGroup
	op1             *rlwe.Ciphertext
	opOut           *rlwe.Ciphertext
	res             *rlwe.Ciphertext
	elapsedmultTask time.Duration
}

var elapsedEncryptParty time.Duration
var elapsedEncryptCloud time.Duration
var elapsedCKGCloud time.Duration
var elapsedCKGParty time.Duration
var elapsedRKGCloud time.Duration
var elapsedRKGParty time.Duration
var elapsedPCKSCloud time.Duration
var elapsedPCKSParty time.Duration
var elapsedEvalCloudCPU time.Duration
var elapsedEvalCloud time.Duration
var elapsedEvalParty time.Duration

func main() {
	// For more details about the PSI example see
	//     Multiparty Homomorphic Encryption: From Theory to Practice (<https://eprint.iacr.org/2020/304>)

	l := log.New(os.Stderr, "", 0)

	// $go run main.go arg1 arg2
	// arg1: number of parties
	// arg2: number of Go routines

	// Largest for n=8192: 512 parties
	N := 2 // Default number of parties
	var err error
	if len(os.Args[1:]) >= 1 {
		N, err = strconv.Atoi(os.Args[1])
		check(err)
	}

	NGoRoutine := 1 // Default number of Go routines
	if len(os.Args[1:]) >= 2 {
		NGoRoutine, err = strconv.Atoi(os.Args[2])
		check(err)
	}

	LogN := 14 // LogN
	if len(os.Args[1:]) >= 3 {
		LogN, err = strconv.Atoi(os.Args[3])
		check(err)
	}

	PlaintextModulus := uint64(65537) // PlaintextModulus
	if len(os.Args[1:]) >= 4 {
		PlaintextModulus, err = strconv.ParseUint(os.Args[4], 10, 64)
		check(err)
	}

	data_size := "1MB" // number of MB
	if len(os.Args[1:]) >= 5 {
		data_size = os.Args[5]
		check(err)
	}

	// Creating encryption parameters from a default params with logN=14, logQP=438 with a plaintext modulus T=65537
	params, err := bgv.NewParametersFromLiteral(bgv.ParametersLiteral{
		LogN:             LogN,
		LogQ:             []int{56, 55, 55, 54, 54, 54},
		LogP:             []int{55, 55},
		PlaintextModulus: PlaintextModulus,
	})
	if err != nil {
		panic(err)
	}

	crs, err := sampling.NewKeyedPRNG([]byte{'l', 'a', 't', 't', 'i', 'g', 'o'})
	if err != nil {
		panic(err)
	}

	encoder := bgv.NewEncoder(params)

	// Target private and public keys
	tsk, tpk := rlwe.NewKeyGenerator(params).GenKeyPairNew()

	// Create each party, and allocate the memory for all the shares that the protocols will need
	P := genparties(params, N)

	// Inputs & expected result
	expRes := genTPCHInputs(data_size, params, P)

	// 1) Collective public key generation
	pk := ckgphase(params, crs, P)

	// 2) Collective relinearization key generation
	rlk := rkgphase(params, crs, P)

	evk := rlwe.NewMemEvaluationKeySet(rlk)

	l.Printf("\tdone (cloud: %s, party: %s)\n",
		elapsedRKGCloud, elapsedRKGParty)
	l.Printf("\tSetup done (cloud: %s, party: %s)\n",
		elapsedRKGCloud+elapsedCKGCloud, elapsedRKGParty+elapsedCKGParty)

	encInputs := encPhase(params, P, pk, encoder)

	encRes := evalPhase(params, NGoRoutine, encInputs, evk)

	encOut := pcksPhase(params, tpk, encRes, P)

	// Decrypt the result with the target secret key
	l.Println("> ResulPlaintextModulus:")
	decryptor := rlwe.NewDecryptor(params, tsk)
	ptres := bgv.NewPlaintext(params, params.MaxLevel())
	elapsedDecParty := runTimed(func() {
		decryptor.Decrypt(encOut, ptres)
	})

	// Check the result
	res := make([]uint64, params.MaxSlots())
	if err := encoder.Decode(ptres, res); err != nil {
		panic(err)
	}
	l.Printf("\t%v\n", res[:16])
	for i := range expRes {
		if expRes[i] != res[i] {
			//l.Printf("\t%v\n", expRes)
			l.Println("\tincorrect")
			return
		}
	}
	l.Println("\tcorrect")
	l.Printf("> Finished (total cloud: %s, total party: %s)\n",
		elapsedCKGCloud+elapsedRKGCloud+elapsedEncryptCloud+elapsedEvalCloud+elapsedPCKSCloud,
		elapsedCKGParty+elapsedRKGParty+elapsedEncryptParty+elapsedEvalParty+elapsedPCKSParty+elapsedDecParty)

}

func encPhase(params bgv.Parameters, P []*party, pk *rlwe.PublicKey, encoder *bgv.Encoder) (encInputs []*rlwe.Ciphertext) {

	l := log.New(os.Stderr, "", 0)

	encInputs = make([]*rlwe.Ciphertext, len(P))
	for i := range encInputs {
		encInputs[i] = bgv.NewCiphertext(params, 1, params.MaxLevel())
	}

	// Each party encrypts its input vector
	l.Println("> Encrypt Phase")
	encryptor := rlwe.NewEncryptor(params, pk)

	pt := bgv.NewPlaintext(params, params.MaxLevel())
	elapsedEncryptParty = runTimedParty(func() {
		for i, pi := range P {
			if err := encoder.Encode(pi.input, pt); err != nil {
				panic(err)
			}
			if err := encryptor.Encrypt(pt, encInputs[i]); err != nil {
				panic(err)
			}
		}
	}, len(P))

	elapsedEncryptCloud = time.Duration(0)
	l.Printf("\tdone (cloud: %s, party: %s)\n", elapsedEncryptCloud, elapsedEncryptParty)

	return
}

func evalPhase(params bgv.Parameters, NGoRoutine int, encInputs []*rlwe.Ciphertext, evk rlwe.EvaluationKeySet) (encRes *rlwe.Ciphertext) {

	l := log.New(os.Stderr, "", 0)

	encLvls := make([][]*rlwe.Ciphertext, 0)
	encLvls = append(encLvls, encInputs)
	for nLvl := len(encInputs) / 2; nLvl > 0; nLvl = nLvl >> 1 {
		encLvl := make([]*rlwe.Ciphertext, nLvl)
		for i := range encLvl {
			encLvl[i] = bgv.NewCiphertext(params, 2, params.MaxLevel())
		}
		encLvls = append(encLvls, encLvl)
	}
	encRes = encLvls[len(encLvls)-1][0]

	evaluator := bgv.NewEvaluator(params, evk)
	// Split the task among the Go routines
	tasks := make(chan *multTask)
	workers := &sync.WaitGroup{}
	workers.Add(NGoRoutine)
	//l.Println("> Spawning", NGoRoutine, "evaluator goroutine")
	for i := 1; i <= NGoRoutine; i++ {
		go func(i int) {
			evaluator := evaluator.ShallowCopy() // creates a shallow evaluator copy for this goroutine
			for task := range tasks {
				task.elapsedmultTask = runTimed(func() {
					// 1) Multiplication of two input vectors
					if err := evaluator.Mul(task.op1, task.opOut, task.res); err != nil {
						panic(err)
					}
					// 2) Relinearization
					if err := evaluator.Relinearize(task.res, task.res); err != nil {
						panic(err)
					}
				})
				task.wg.Done()
			}
			//l.Println("\t evaluator", i, "down")
			workers.Done()
		}(i)
		//l.Println("\t evaluator", i, "started")
	}

	// Start the tasks
	taskList := make([]*multTask, 0)
	l.Println("> Eval Phase")
	elapsedEvalCloud = runTimed(func() {
		for i, lvl := range encLvls[:len(encLvls)-1] {
			nextLvl := encLvls[i+1]
			l.Println("\tlevel", i, len(lvl), "->", len(nextLvl))
			wg := &sync.WaitGroup{}
			wg.Add(len(nextLvl))
			for j, nextLvlCt := range nextLvl {
				task := multTask{wg, lvl[2*j], lvl[2*j+1], nextLvlCt, 0}
				taskList = append(taskList, &task)
				tasks <- &task
			}
			wg.Wait()
		}
	})
	elapsedEvalCloudCPU = time.Duration(0)
	for _, t := range taskList {
		elapsedEvalCloudCPU += t.elapsedmultTask
	}
	elapsedEvalParty = time.Duration(0)
	l.Printf("\tdone (cloud: %s (wall: %s), party: %s)\n",
		elapsedEvalCloudCPU, elapsedEvalCloud, elapsedEvalParty)

	//l.Println("> Shutting down workers")
	close(tasks)
	workers.Wait()

	return
}

func genparties(params bgv.Parameters, N int) []*party {

	// Create each party, and allocate the memory for all the shares that the protocols will need
	P := make([]*party, N)
	for i := range P {
		pi := &party{}
		pi.sk = rlwe.NewKeyGenerator(params).GenSecretKeyNew()

		P[i] = pi
	}

	return P
}

func genInputs(params bgv.Parameters, P []*party) (expRes []uint64) {

	expRes = make([]uint64, params.N())
	for i := range expRes {
		expRes[i] = 1
	}

	for _, pi := range P {

		pi.input = make([]uint64, params.N())
		for i := range pi.input {
			if sampling.RandFloat64(0, 1) > 0.3 || i == 4 {
				pi.input[i] = 1
			}
			expRes[i] *= pi.input[i]
		}

	}

	return
}

func genTPCHInputs(data_size string, params bgv.Parameters, P []*party) (expRes []uint64) {

	expRes = make([]uint64, params.N())
	for i := range expRes {
		expRes[i] = 1
	}

	for i, pi := range P {

		encoded, err := OneHotEncodeCSV("../tpch_workdir/"+data_size+"/split0.5/orders"+strconv.Itoa(i+1)+".csv", params.N())
		if err != nil {
			panic(err)
		}
		pi.input = encoded
		for i := range pi.input {
			expRes[i] *= pi.input[i]
		}
	}

	return
}

// OneHotEncodeCSV reads a CSV file with a single column of numbers and returns a one-hot encoded array.
func OneHotEncodeCSV(filePath string, N int) ([]uint64, error) {
	// Open the CSV file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Create a new CSV reader
	reader := csv.NewReader(file)

	// Read all records from the CSV file
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV data: %w", err)
	}

	// Create an array to hold the one-hot encoded data
	oneHotEncodedArray := make([]uint64, N)

	// Iterate over each record in the CSV file
	for _, record := range records {
		for _, value := range record {
			num, err := strconv.Atoi(value)
			if err != nil {
				log.Printf("Skipping invalid number: %s", value)
				continue
			}

			if num >= 0 && num < N {
				oneHotEncodedArray[num] = 1
			} else {
				log.Printf("Number %d is out of range for one-hot encoding", num)
			}
		}
	}

	return oneHotEncodedArray, nil
}

func pcksPhase(params bgv.Parameters, tpk *rlwe.PublicKey, encRes *rlwe.Ciphertext, P []*party) (encOut *rlwe.Ciphertext) {

	l := log.New(os.Stderr, "", 0)

	// Collective key switching from the collective secret key to
	// the target public key

	pcks, err := multiparty.NewPublicKeySwitchProtocol(params, ring.DiscreteGaussian{Sigma: 1 << 30, Bound: 6 * (1 << 30)})
	if err != nil {
		panic(err)
	}

	for _, pi := range P {
		pi.pcksShare = pcks.AllocateShare(params.MaxLevel())
	}

	l.Println("> PublicKeySwitch Phase")
	elapsedPCKSParty = runTimedParty(func() {
		for _, pi := range P {
			/* #nosec G601 -- Implicit memory aliasing in for loop acknowledged */
			pcks.GenShare(pi.sk, tpk, encRes, &pi.pcksShare)
		}
	}, len(P))

	pcksCombined := pcks.AllocateShare(params.MaxLevel())
	encOut = bgv.NewCiphertext(params, 1, params.MaxLevel())
	elapsedPCKSCloud = runTimed(func() {
		for _, pi := range P {
			if err = pcks.AggregateShares(pi.pcksShare, pcksCombined, &pcksCombined); err != nil {
				panic(err)
			}
		}

		pcks.KeySwitch(encRes, pcksCombined, encOut)
	})
	l.Printf("\tdone (cloud: %s, party: %s)\n", elapsedPCKSCloud, elapsedPCKSParty)

	return
}

func rkgphase(params bgv.Parameters, crs sampling.PRNG, P []*party) *rlwe.RelinearizationKey {
	l := log.New(os.Stderr, "", 0)

	l.Println("> RelinearizationKeyGen Phase")

	rkg := multiparty.NewRelinearizationKeyGenProtocol(params) // Relineariation key generation
	_, rkgCombined1, rkgCombined2 := rkg.AllocateShare()

	for _, pi := range P {
		pi.rlkEphemSk, pi.rkgShareOne, pi.rkgShareTwo = rkg.AllocateShare()
	}

	crp := rkg.SampleCRP(crs)

	elapsedRKGParty = runTimedParty(func() {
		for _, pi := range P {
			/* #nosec G601 -- Implicit memory aliasing in for loop acknowledged */
			rkg.GenShareRoundOne(pi.sk, crp, pi.rlkEphemSk, &pi.rkgShareOne)
		}
	}, len(P))

	elapsedRKGCloud = runTimed(func() {
		for _, pi := range P {
			/* #nosec G601 -- Implicit memory aliasing in for loop acknowledged */
			rkg.AggregateShares(pi.rkgShareOne, rkgCombined1, &rkgCombined1)
		}
	})

	elapsedRKGParty += runTimedParty(func() {
		for _, pi := range P {
			/* #nosec G601 -- Implicit memory aliasing in for loop acknowledged */
			rkg.GenShareRoundTwo(pi.rlkEphemSk, pi.sk, rkgCombined1, &pi.rkgShareTwo)
		}
	}, len(P))

	rlk := rlwe.NewRelinearizationKey(params)
	elapsedRKGCloud += runTimed(func() {
		for _, pi := range P {
			/* #nosec G601 -- Implicit memory aliasing in for loop acknowledged */
			rkg.AggregateShares(pi.rkgShareTwo, rkgCombined2, &rkgCombined2)
		}
		rkg.GenRelinearizationKey(rkgCombined1, rkgCombined2, rlk)
	})

	l.Printf("\tdone (cloud: %s, party: %s)\n", elapsedRKGCloud, elapsedRKGParty)

	return rlk
}

func ckgphase(params bgv.Parameters, crs sampling.PRNG, P []*party) *rlwe.PublicKey {

	l := log.New(os.Stderr, "", 0)

	l.Println("> PublicKeyGen Phase")

	ckg := multiparty.NewPublicKeyGenProtocol(params) // Public key generation
	ckgCombined := ckg.AllocateShare()
	for _, pi := range P {
		pi.ckgShare = ckg.AllocateShare()
	}

	crp := ckg.SampleCRP(crs)

	elapsedCKGParty = runTimedParty(func() {
		for _, pi := range P {
			/* #nosec G601 -- Implicit memory aliasing in for loop acknowledged */
			ckg.GenShare(pi.sk, crp, &pi.ckgShare)
		}
	}, len(P))

	pk := rlwe.NewPublicKey(params)

	elapsedCKGCloud = runTimed(func() {
		for _, pi := range P {
			ckg.AggregateShares(pi.ckgShare, ckgCombined, &ckgCombined)
		}
		ckg.GenPublicKey(ckgCombined, crp, pk)
	})

	l.Printf("\tdone (cloud: %s, party: %s)\n", elapsedCKGCloud, elapsedCKGParty)

	return pk
}
