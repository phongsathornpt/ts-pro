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
