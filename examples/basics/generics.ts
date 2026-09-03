function identity<T>(x: T): T {
  return x;
}

function pair<A, B>(first: A, second: B): [A, B] {
  return [first, second];
}

class Box<T> {
  value: T;
  constructor(v: T) {
    this.value = v;
  }
  get(): T {
    return this.value;
  }
}

class Stack<T> {
  items: T[];
  constructor() {
    this.items = [];
  }
  pushItem(item: T): void {
    this.items.push(item);
  }
  popItem(): T | undefined {
    return this.items.pop();
  }
  size(): number {
    return this.items.length;
  }
}

export function testGenerics(): void {
  console.log(identity<number>(42));
  console.log(identity<string>("hello"));

  const p = pair<string, number>("answer", 42);
  console.log(p[0]);
  console.log(p[1]);

  const b1 = new Box<number>(100);
  console.log(b1.get());
  const b2 = new Box<string>("boxed");
  console.log(b2.get());

  const s = new Stack<number>();
  s.pushItem(10);
  s.pushItem(20);
  s.pushItem(30);
  console.log(s.size());
  console.log(s.popItem());
  console.log(s.popItem());
  console.log(s.size());
}
testGenerics();
