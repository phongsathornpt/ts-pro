export function testMap(): void {
  const m = new Map<string, number>();
  m.set("apple", 10);
  m.set("banana", 20);
  console.log(m.get("apple"));
  console.log(m.get("banana"));
  console.log(m.has("apple"));
  console.log(m.has("cherry"));
  console.log(m.size);
  m.delete("apple");
  console.log(m.has("apple"));
  console.log(m.size);
  m.clear();
  console.log(m.size);
}

export function testSet(): void {
  const s = new Set<string>();
  s.add("alpha");
  s.add("beta");
  console.log(s.has("alpha"));
  console.log(s.has("gamma"));
  console.log(s.size);
  s.delete("alpha");
  console.log(s.has("alpha"));
  console.log(s.size);
  s.clear();
  console.log(s.size);
}

export function testChaining(): void {
  const m = new Map<string, number>();
  m.set("k1", 100).set("k2", 200);
  console.log(m.size);
  console.log(m.get("k1"));
  console.log(m.get("k2"));

  const s = new Set<number>();
  s.add(1).add(2).add(3);
  console.log(s.size);
}

testMap();
testSet();
testChaining();
