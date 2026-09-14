package e2e_test

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

type runtimeStressRNG uint64

func (r *runtimeStressRNG) next() uint64 {
	x := uint64(*r)
	x ^= x << 13
	x ^= x >> 7
	x ^= x << 17
	*r = runtimeStressRNG(x)
	return x
}

func (r *runtimeStressRNG) n(limit int) int {
	return int(r.next() % uint64(limit))
}

func runtimeStressSeeds(t *testing.T) []uint64 {
	t.Helper()
	seeds := []uint64{1, 7, 42, 1337, 0x5eed5eed}
	if raw := os.Getenv("TS_PRO_STRESS_SEED"); raw != "" {
		seed, err := strconv.ParseUint(raw, 0, 64)
		if err != nil {
			t.Fatalf("invalid TS_PRO_STRESS_SEED %q: %v", raw, err)
		}
		if seed == 0 {
			seed = 1
		}
		seeds = append(seeds, seed)
	}
	return seeds
}

func buildRuntimeStressProgram(seed uint64) (string, string) {
	rng := runtimeStressRNG(seed)
	workers := 6 + rng.n(5)
	valueCapacity := rng.n(4)
	labelCapacity := rng.n(3)

	var src strings.Builder
	fmt.Fprintf(&src, "const values = channel<number>(%d);\n", valueCapacity)
	fmt.Fprintf(&src, "const labels = channel<string>(%d);\n", labelCapacity)

	for i := 0; i < workers; i++ {
		value := i + 1
		churn := 2500 + rng.n(2500)
		fmt.Fprintf(&src, "const task%d = spawn((): number => {\n", i)
		fmt.Fprintf(&src, "  const keep = \"worker-%d-\" + \"alive\";\n", i)
		for step := 0; step < 1+rng.n(4); step++ {
			if rng.n(2) == 0 {
				fmt.Fprintln(&src, "  yieldNow();")
			} else {
				fmt.Fprintln(&src, "  sleep(0);")
			}
		}
		fmt.Fprintln(&src, "  let churn = \"\";")
		fmt.Fprintf(&src, "  for (let i = 0; i < %d; i = i + 1) { churn = \"gc-\" + keep; }\n", churn)
		// Always publish the numeric value first. With two rendezvous channels,
		// randomizing send order can create a test-harness deadlock where the
		// main task waits on values while every worker waits on labels.
		fmt.Fprintf(&src, "  channelSend(values, %d);\n", value)
		fmt.Fprintln(&src, "  channelSend(labels, keep);")
		fmt.Fprintf(&src, "  return %d;\n});\n", value*2)
	}

	fmt.Fprintln(&src, "let valueSum = 0;")
	fmt.Fprintln(&src, "let labelCount = 0;")
	fmt.Fprintf(&src, "for (let i = 0; i < %d; i = i + 1) { valueSum += channelRecv(values); }\n", workers)
	fmt.Fprintf(&src, "for (let i = 0; i < %d; i = i + 1) { const label = channelRecv(labels); if (label != \"\") { labelCount += 1; } }\n", workers)
	fmt.Fprintln(&src, "let joinSum = 0;")
	for i := 0; i < workers; i++ {
		fmt.Fprintf(&src, "joinSum += join(task%d);\n", i)
	}
	fmt.Fprintln(&src, "console.log(valueSum);")
	fmt.Fprintln(&src, "console.log(labelCount);")
	fmt.Fprintln(&src, "console.log(joinSum);")

	promiseCount := 4 + rng.n(5)
	for i := 0; i < promiseCount; i++ {
		value := (i + 1) * 3
		churn := 1200 + rng.n(1800)
		fmt.Fprintf(&src, "async function promiseWorker%d(): Promise<number> {\n", i)
		for step := 0; step < 1+rng.n(3); step++ {
			if rng.n(2) == 0 {
				fmt.Fprintln(&src, "  yieldNow();")
			} else {
				fmt.Fprintln(&src, "  sleep(0);")
			}
		}
		fmt.Fprintf(&src, "  const held = \"promise-%d-\" + \"alive\";\n", i)
		fmt.Fprintln(&src, "  let churn = \"\";")
		fmt.Fprintf(&src, "  for (let i = 0; i < %d; i = i + 1) { churn = held + \"-gc\"; }\n", churn)
		fmt.Fprintf(&src, "  return await Promise.resolve(%d);\n}\n", value)
	}
	fmt.Fprintln(&src, "async function promiseStress(): Promise<number> {")
	fmt.Fprint(&src, "  const resolved = await Promise.all<number>([")
	for i := 0; i < promiseCount; i++ {
		if i > 0 {
			fmt.Fprint(&src, ", ")
		}
		fmt.Fprintf(&src, "promiseWorker%d()", i)
	}
	fmt.Fprintln(&src, "]);")
	fmt.Fprintln(&src, "  let total = 0;")
	fmt.Fprintln(&src, "  for (const value of resolved) { total += value; }")
	fmt.Fprintln(&src, "  return total;")
	fmt.Fprintln(&src, "}")
	fmt.Fprintln(&src, "console.log(join(promiseStress()));")

	valueSum := workers * (workers + 1) / 2
	joinSum := valueSum * 2
	promiseSum := 3 * promiseCount * (promiseCount + 1) / 2
	expected := fmt.Sprintf("%d\n%d\n%d\n%d\n", valueSum, workers, joinSum, promiseSum)
	return src.String(), expected
}

func TestLinuxAMD64DeterministicRandomizedRuntimeStress(t *testing.T) {
	for _, seed := range runtimeStressSeeds(t) {
		t.Run(fmt.Sprintf("seed_%d", seed), func(t *testing.T) {
			source, expected := buildRuntimeStressProgram(seed)
			runLinuxAMD64(t, linuxAMD64Case{
				name:     fmt.Sprintf("runtime_stress_%d", seed),
				source:   source,
				expected: expected,
			})
		})
	}
}
