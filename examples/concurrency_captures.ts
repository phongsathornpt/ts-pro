function churnTaskGC(n: number): string {
  let value: string = "";
  for (let i = 0; i < n; i++) {
    value = value + "x";
  }
  return "ready";
}

const message: string = "captured";
const capturedTask = spawn((): void => {
  console.log(message);
});
churnTaskGC(20000);
join(capturedTask);
console.log("done");
