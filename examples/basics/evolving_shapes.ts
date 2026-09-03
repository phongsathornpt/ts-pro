export function testEvolvingShapes(): void {
  const o1: any = {};
  o1.x = 10;
  o1.y = 20;
  console.log(o1.x);
  console.log(o1.y);

  const o2: any = { a: 1 };
  o2.b = "added";
  o2["c"] = 42;
  o2.a = 99;
  console.log(o2.a);
  console.log(o2.b);
  console.log(o2.c);
  console.log(o2.missing);
}
testEvolvingShapes();
