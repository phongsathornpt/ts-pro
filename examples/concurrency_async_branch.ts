async function asyncChoose(flag: boolean): Promise<number> {
  sleep(1);
  if (flag) return 42;
  return 7;
}

console.log(join(asyncChoose(true)));
console.log(join(asyncChoose(false)));
