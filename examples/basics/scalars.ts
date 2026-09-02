export function math(a: number, b: number): number {
  return a * b + a / b;
}

export function compare(a: number, b: number): number {
  if (a < b) return 1;
  if (a >= b) return 2;
  if (a == b) return 3;
  if (a != b) return 4;
  return 5;
}

console.log(math(8, 2));
console.log(compare(1, 2));
