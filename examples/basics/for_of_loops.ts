export function sumArray(nums: number[]): number {
  let total = 0;
  for (const n of nums) {
    total = total + n;
  }
  return total;
}

export function joinWords(words: string[]): string {
  let res = "";
  for (const w of words) {
    res = `${res}${w} `;
  }
  return res;
}

export function countMatches(flags: boolean[]): number {
  let count = 0;
  for (const f of flags) {
    if (f) {
      count = count + 1;
    }
  }
  return count;
}

// 1. Numeric array iteration
const numbers = [10, 20, 30, 40];
console.log(sumArray(numbers));

// 2. String array iteration
const words = ["ts-pro", "is", "fast", "and", "pure-Go"];
console.log(joinWords(words));

// 3. Boolean array iteration
const flags = [true, false, true, true, false];
console.log(countMatches(flags));

// 4. Top-level for...of loop
for (const x of numbers) {
  console.log(x);
}
