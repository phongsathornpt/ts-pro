async function aggregateDelayed(value: number): Promise<number> {
  sleep(1);
  return value;
}

async function aggregateDelay(milliseconds: number, value: number): Promise<number> {
  sleep(milliseconds);
  return value;
}

async function aggregateAll(): Promise<number> {
  const values = await Promise.all<number>([
    Promise.resolve(3),
    aggregateDelayed(4),
    Promise.resolve(5),
  ]);
  return values[0]! * 100 + values[1]! * 10 + values[2]!;
}

async function aggregateRace(): Promise<number> {
  return await Promise.race<number>([
    Promise.resolve(7),
    Promise.resolve(9),
  ]);
}

async function aggregateReject(): Promise<number> {
  try {
    await Promise.all<number>([
      Promise.resolve(1),
      Promise.reject<number>("aggregate-reject"),
      aggregateDelayed(3),
    ]);
    return 0;
  } catch (error: any) {
    console.log(error);
    return 42;
  }
}

console.log(join(aggregateAll()));
console.log(join(aggregateRace()));
console.log(join(aggregateReject()));

async function aggregateAliasReuse(): Promise<number> {
  const first: Promise<number> = aggregateDelayed(2);
  const second: Promise<number> = aggregateDelayed(5);
  const values = await Promise.all<number>([first, second]);
  const repeated = await first;
  return values[0]! + values[1]! + repeated;
}

async function aggregateRaceAliasReuse(): Promise<number> {
  const first: Promise<number> = Promise.resolve(13);
  const second: Promise<number> = aggregateDelayed(17);
  const raced = await Promise.race<number>([first, second]);
  const loser = await second;
  return raced + loser;
}

console.log(join(aggregateAliasReuse()));
console.log(join(aggregateRaceAliasReuse()));

async function aggregatePendingRace(): Promise<number> {
  return await Promise.race<number>([
    aggregateDelay(8, 20),
    aggregateDelay(1, 30),
  ]);
}

async function aggregateRaceReject(): Promise<number> {
  try {
    await Promise.race<number>([
      Promise.reject<number>("race-reject"),
      aggregateDelay(5, 99),
    ]);
    return 0;
  } catch (error: any) {
    console.log(error);
    return 43;
  }
}

console.log(join(aggregatePendingRace()));
console.log(join(aggregateRaceReject()));

async function aggregateDelayedString(value: string): Promise<string> {
  sleep(1);
  return value + "-done";
}

async function aggregateAllStrings(): Promise<string> {
  const values = await Promise.all<string>([
    Promise.resolve("alpha"),
    aggregateDelayedString("beta"),
  ]);
  return values[0]! + ":" + values[1]!;
}

async function aggregateRaceStrings(): Promise<string> {
  return await Promise.race<string>([
    Promise.resolve("first-root"),
    aggregateDelayedString("second-root"),
  ]);
}

console.log(join(aggregateAllStrings()));
console.log(join(aggregateRaceStrings()));

function aggregateBoolScore(value: boolean): number {
  if (value) return 1;
  return 0;
}

async function aggregateAllBooleans(): Promise<number> {
  const values = await Promise.all<boolean>([
    Promise.resolve(true),
    Promise.resolve(false),
    Promise.resolve(true),
  ]);
  return aggregateBoolScore(values[0]!) * 100 + aggregateBoolScore(values[1]!) * 10 + aggregateBoolScore(values[2]!);
}

async function aggregateRaceBooleans(): Promise<number> {
  const value = await Promise.race<boolean>([
    Promise.resolve(false),
    Promise.resolve(true),
  ]);
  return aggregateBoolScore(value);
}

console.log(join(aggregateAllBooleans()));
console.log(join(aggregateRaceBooleans()));

async function aggregateDelayedBoolean(milliseconds: number, value: boolean): Promise<boolean> {
  sleep(milliseconds);
  return value;
}

async function aggregatePendingRaceBooleans(): Promise<number> {
  const value = await Promise.race<boolean>([
    aggregateDelayedBoolean(8, false),
    aggregateDelayedBoolean(1, true),
  ]);
  return aggregateBoolScore(value);
}

async function aggregateRejectBooleans(): Promise<number> {
  try {
    await Promise.all<boolean>([
      Promise.resolve(true),
      Promise.reject<boolean>("bool-reject"),
      aggregateDelayedBoolean(3, true),
    ]);
    return 0;
  } catch (error: any) {
    console.log(error);
    return 44;
  }
}

console.log(join(aggregatePendingRaceBooleans()));
console.log(join(aggregateRejectBooleans()));

async function aggregateMixedRawNumbers(): Promise<number> {
  const values = await Promise.all<number>([
    1,
    Promise.resolve(2),
    3,
  ]);
  return values[0]! * 100 + values[1]! * 10 + values[2]!;
}

async function aggregateRaceRawNumber(): Promise<number> {
  return await Promise.race<number>([
    8,
    aggregateDelay(5, 9),
  ]);
}

async function aggregateMixedRawStrings(): Promise<string> {
  const values = await Promise.all<string>([
    "raw",
    Promise.resolve("promise"),
  ]);
  return values[0]! + ":" + values[1]!;
}

async function aggregateMixedRawBooleans(): Promise<number> {
  const values = await Promise.all<boolean>([
    true,
    Promise.resolve(false),
  ]);
  return aggregateBoolScore(values[0]!) * 10 + aggregateBoolScore(values[1]!);
}

console.log(join(aggregateMixedRawNumbers()));
console.log(join(aggregateRaceRawNumber()));
console.log(join(aggregateMixedRawStrings()));
console.log(join(aggregateMixedRawBooleans()));
