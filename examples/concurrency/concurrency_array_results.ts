const taskArrayResult = spawn((): number[] => [1, 42, 3]);
const taskArrayValue: number[] = join(taskArrayResult);
console.log(taskArrayValue[1]!);
