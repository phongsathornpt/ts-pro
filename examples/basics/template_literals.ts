export function formatGreeting(name: string, age: number): string {
  return `Hello, ${name}! Next year you will be ${age + 1}.`;
}

export function formatStatus(active: boolean, count: number): string {
  return `status: ${active}, count: ${count}`;
}

export function formatPlain(): string {
  return `simple template with no substitutions`;
}

console.log(formatGreeting("Alice", 25));
console.log(formatGreeting("Bob", 30));
console.log(formatStatus(true, 10));
console.log(formatStatus(false, 0));
console.log(formatPlain());

for (let i = 1; i <= 3; i++) {
  console.log(`item #${i}: square=${i * i}`);
}
