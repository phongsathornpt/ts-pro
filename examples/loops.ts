export function sumWhile(n: number): number {
  let total: number = 0;
  let i: number = 0;
  while (i < n) {
    total = total + i;
    i++;
  }
  return total;
}

export function sumFor(n: number): number {
  let total: number = 0;
  for (let i: number = 0; i < n; i++) {
    total = total + i;
  }
  return total;
}

console.log(sumWhile(10));
console.log(sumFor(10));
