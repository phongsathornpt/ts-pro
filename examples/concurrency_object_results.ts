const taskObjectResult = spawn((): { value: number } => ({ value: 42 }));
const taskObjectValue: { value: number } = join(taskObjectResult);
console.log(taskObjectValue.value);
