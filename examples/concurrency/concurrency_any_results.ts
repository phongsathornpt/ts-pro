const taskAnyResult = spawn((): any => 42);
const taskAnyValue: any = join(taskAnyResult);
console.log(taskAnyValue);
