// 1. Rest parameters
export function sumAll(...nums: number[]): number {
  let total = 0;
  for (const n of nums) {
    total = total + n;
  }
  return total;
}

export function formatList(prefix: string, ...items: string[]): string {
  let res = prefix;
  for (const item of items) {
    res = `${res}:${item}`;
  }
  return res;
}

console.log(sumAll(10, 20, 30));
console.log(sumAll(1, 2, 3, 4, 5));
console.log(sumAll());
console.log(formatList("items", "apple", "banana", "cherry"));
console.log(formatList("empty"));

// 2. Array spread
export function testArraySpread(): void {
  const a = [10, 20];
  const b = [0, ...a, 30, 40];
  for (const x of b) {
    console.log(x);
  }

  const s1 = ["first", "second"];
  const s2 = ["third"];
  const combined = [...s1, ...s2];
  for (const s of combined) {
    console.log(s);
  }
}
testArraySpread();

// 3. Object spread
export function testObjectSpread(): void {
  const defaults = { host: "localhost", port: 8080, secure: false };
  const custom = { port: 9000, secure: true };
  const config = { ...defaults, ...custom, env: "prod" };
  console.log(config.host);
  console.log(config.port);
  console.log(config.secure);
  console.log(config.env);
}
testObjectSpread();
