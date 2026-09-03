package e2e_test

import "testing"

func TestE2E_Fibonacci(t *testing.T) {
	runHappy(t, happyCase{
		name: "recursion_fibonacci",
		source: `
function fib(n: number): number {
	if (n <= 1) {
		return n;
	}
	return fib(n - 1) + fib(n - 2);
}
console.log(fib(10));
console.log(fib(15));
`,
		expected: "55\n610\n",
	})
}

func TestE2E_Factorial(t *testing.T) {
	runHappy(t, happyCase{
		name: "recursion_factorial",
		source: `
function fact(n: number): number {
	if (n <= 1) {
		return 1;
	}
	return n * fact(n - 1);
}
console.log(fact(6));
`,
		expected: "720\n",
	})
}

func TestE2E_GCD(t *testing.T) {
	runHappy(t, happyCase{
		name: "recursion_gcd",
		source: `
function gcd(a: number, b: number): number {
	if (b == 0) {
		return a;
	}
	return gcd(b, a % b);
}
console.log(gcd(48, 18));
console.log(gcd(101, 10));
`,
		expected: "6\n1\n",
	})
}
