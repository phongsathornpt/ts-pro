const structuredGroup = taskGroup();
const structuredChannel = channel<number>(2);
const structuredA = groupSpawn(structuredGroup, (): number => {
  channelSend(structuredChannel, 1);
  return 10;
});
const structuredB = groupSpawn(structuredGroup, (): number => {
  channelSend(structuredChannel, 2);
  return 20;
});
groupJoin(structuredGroup);
console.log(channelRecv(structuredChannel));
console.log(channelRecv(structuredChannel));
console.log(join(structuredA) + join(structuredB));
