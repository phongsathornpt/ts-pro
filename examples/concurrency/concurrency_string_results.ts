const taskStringBase = "task";
const taskStringResult = spawn((): string => taskStringBase + "-string");
const taskStringValue: string = join(taskStringResult);
console.log(taskStringValue);
