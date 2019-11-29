package bfv

import (
	"github.com/ldsec/lattigo/ring"
)

// genModuli generates the appropriate primes from the parameters using generateCKKSPrimes such that all primes are different.
func genModuli(params *Parameters) (Q1 []uint64, P []uint64, Q2 []uint64) {

	// Extracts all the different primes bit size and maps their number
	primesbitlen := make(map[uint64]uint64)
	for i, qi := range params.Q1 {

		primesbitlen[uint64(qi)]++

		if uint64(params.Q1[i]) > 60 {
			panic("provided moduli must be smaller than 61")
		}
	}

	for _, pj := range params.P {
		primesbitlen[uint64(pj)]++

		if uint64(pj) > 60 {
			panic("provided P must be smaller than 61")
		}
	}

	for i, qi := range params.Q2 {

		primesbitlen[uint64(qi)]++

		if uint64(params.Q2[i]) > 60 {
			panic("provided moduli must be smaller than 61")
		}
	}

	// For each bitsize, finds that many primes
	primes := make(map[uint64][]uint64)
	for key, value := range primesbitlen {
		primes[key] = generateNTTPrimes(key, uint64(params.LogN), value)
	}

	// Assigns the primes to the ckks moduli chain
	Q1 = make([]uint64, len(params.Q1))
	for i, qi := range params.Q1 {
		Q1[i] = primes[uint64(params.Q1[i])][0]
		primes[uint64(qi)] = primes[uint64(qi)][1:]
	}

	// Assigns the primes to the special primes list for the the keyscontext
	P = make([]uint64, len(params.P))
	for i, pj := range params.P {
		P[i] = primes[uint64(pj)][0]
		primes[uint64(pj)] = primes[uint64(pj)][1:]
	}

	Q2 = make([]uint64, len(params.Q2))
	for i, qi := range params.Q2 {
		Q2[i] = primes[uint64(params.Q2[i])][0]
		primes[uint64(qi)] = primes[uint64(qi)][1:]
	}

	return Q1, P, Q2
}

func generateNTTPrimes(logQ, logN, levels uint64) (primes []uint64) {

	// generateCKKSPrimes generates primes given logQ = size of the primes, logN = size of N and level, the number
	// of levels required. Will return all the appropriate primes, up to the number of level, with the
	// best avaliable deviation from the base power of 2 for the given level.

	if logQ > 60 {
		panic("logQ must be between 1 and 60")
	}

	var x, y, Qpow2, _2N uint64

	primes = []uint64{}

	Qpow2 = 1 << logQ

	_2N = 2 << logN

	x = Qpow2 + 1
	y = Qpow2 + 1

	for true {

		if ring.IsPrime(y) {
			primes = append(primes, y)
			if uint64(len(primes)) == levels {
				return primes
			}
		}

		y -= _2N

		if ring.IsPrime(x) {
			primes = append(primes, x)
			if uint64(len(primes)) == levels {
				return primes
			}
		}

		x += _2N
	}

	return
}
