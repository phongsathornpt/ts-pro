const resultBase: number = 40;
const resultTask = spawn((): number => resultBase + 2);
const resultValue: number = join(resultTask);
console.log(resultValue);
