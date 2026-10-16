package com.phoring.fenghuangming.player

import kotlin.math.floor

internal class PlaybackGate {
    var blocked = true
    var leaseUntil = 0L
    var epoch = 0L
    var playbackId: String? = null
    fun canPlay(now: Long) = !blocked && now < leaseUntil && playbackId != null
    fun renew(nextEpoch: Long, milliseconds: Long, now: Long) {
        if (nextEpoch < epoch) return
        epoch = nextEpoch
        leaseUntil = now + milliseconds.coerceIn(0, 60_000)
    }
    fun requireTask(id: String, commandEpoch: Long, now: Long) {
        require(canPlay(now) && id == playbackId && commandEpoch == epoch) { "播放任务或连接租约已失效" }
    }
    fun seekMillis(seconds: Double, durationMillis: Long): Long {
        require(seconds.isFinite() && seconds >= 0 && seconds == floor(seconds)) { "进度须为整数秒" }
        val maximum = (durationMillis / 1000 - 1).coerceAtLeast(0)
        require(seconds <= maximum) { "进度超出歌曲时长" }
        return (seconds * 1000).toLong()
    }
}
