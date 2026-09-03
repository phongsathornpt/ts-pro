export function testDate(): void {
  const t = Date.now();
  console.log(t > 0);

  const d1 = new Date(1700000000000);
  console.log(d1.toISOString());
  console.log(d1.getUTCFullYear());
  console.log(d1.getUTCMonth());
  console.log(d1.getUTCDate());
  console.log(d1.getUTCHours());
  console.log(d1.getUTCMinutes());
  console.log(d1.getUTCSeconds());

  const d2 = new Date("2023-11-14T22:13:20.000Z");
  console.log(d2.toISOString());
}
testDate();
