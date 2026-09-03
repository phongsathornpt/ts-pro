import { multiply, power as pow, Counter } from "./modules/helper";

export function main(): void {
  const m = multiply(6, 7);
  console.log(m);

  const p = pow(2, 8);
  console.log(p);

  const c = new Counter(10);
  c.increment();
  c.increment();
  console.log(c.value());
}

main();
