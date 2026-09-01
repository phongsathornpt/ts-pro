const cancelGroup = taskGroup();
const cancelStart = channel<number>(2);
const cancelDone = channel<number>(2);
const cancelA = groupSpawn(cancelGroup, (): void => {
  channelRecv(cancelStart);
  if (taskCancelled()) {
    channelSend(cancelDone, 1);
    return;
  }
  channelSend(cancelDone, 9);
});
const cancelB = groupSpawn(cancelGroup, (): void => {
  channelRecv(cancelStart);
  if (taskCancelled()) {
    channelSend(cancelDone, 2);
    return;
  }
  channelSend(cancelDone, 9);
});
groupCancel(cancelGroup);
channelSend(cancelStart, 1);
channelSend(cancelStart, 1);
groupJoin(cancelGroup);
console.log(channelRecv(cancelDone) + channelRecv(cancelDone));
join(cancelA);
join(cancelB);
