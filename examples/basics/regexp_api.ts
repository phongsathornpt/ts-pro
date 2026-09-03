export function testRegExp(): void {
  const r1 = new RegExp("world", "i");
  console.log(r1.test("Hello World"));
  console.log(r1.test("Hello Earth"));
  console.log(r1.source);

  const r2 = /abc\d+/;
  console.log(r2.test("abc1234"));
  console.log(r2.test("abcdef"));

  const r3 = new RegExp("^foo");
  console.log(r3.test("foobar"));
  console.log(r3.test("barfoo"));
}
testRegExp();
