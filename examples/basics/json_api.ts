export function serializePayload(count: number, key: string): string {
  return JSON.stringify({ count: count, key: key });
}

export function parseAndCheck(text: string): number {
  const parsed = JSON.parse(text);
  if (parsed === 100) {
    return 1;
  }
  return 0;
}

// 1. JSON.stringify with objects
console.log(JSON.stringify({ message: "hello" }));
console.log(JSON.stringify({ status: 200 }));
console.log(serializePayload(5, "items"));

// 2. JSON.stringify with arrays
console.log(JSON.stringify([10, 20, 30]));
console.log(JSON.stringify(["a", "b", "c"]));
console.log(JSON.stringify([true, false]));

// 3. JSON.stringify with scalars
console.log(JSON.stringify(123.45));
console.log(JSON.stringify("pure-Go"));
console.log(JSON.stringify(true));

// 4. JSON.parse with scalars
console.log(JSON.parse("123.45"));
console.log(JSON.parse("\"parsed string\""));
console.log(JSON.parse("true"));
console.log(JSON.parse("false"));

// 5. JSON round-trip
console.log(JSON.stringify(JSON.parse("[1, 2, 3]")));
console.log(JSON.stringify(JSON.parse("{\"id\":42}")));
console.log(parseAndCheck("100"));
