async function asyncSum(limit: number): Promise<number> {
  let total = 0;
  let i = 0;
  while (i < limit) {
    sleep(1);
    total = total + i;
    i++;
  }
  return total;
}

console.log(join(asyncSum(5)));
