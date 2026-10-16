package com.phoring.fenghuangming.player

import org.junit.Assert.*
import org.junit.Test

class PlaybackGateTest {
    @Test fun startsBlockedAndRequiresLease() {
        val gate = PlaybackGate(); gate.playbackId = "play_a"; gate.renew(1, 60000, 0)
        assertFalse(gate.canPlay(1)); gate.blocked = false
        assertTrue(gate.canPlay(59999)); assertFalse(gate.canPlay(60000))
    }
    @Test fun leaseIsCappedAndOldEpochCannotRenew() {
        val gate = PlaybackGate(); gate.renew(2, 900000, 100)
        assertEquals(60100, gate.leaseUntil); gate.renew(1, 60000, 10000)
        assertEquals(60100, gate.leaseUntil)
    }
    @Test fun rejectsOldTaskAndFractionalSeek() {
        val gate = PlaybackGate(); gate.blocked = false; gate.playbackId = "current"; gate.renew(3, 60000, 0)
        assertThrows(IllegalArgumentException::class.java) { gate.requireTask("old", 3, 1) }
        assertThrows(IllegalArgumentException::class.java) { gate.requireTask("current", 2, 1) }
        assertThrows(IllegalArgumentException::class.java) { gate.seekMillis(1.5, 100000) }
        assertThrows(IllegalArgumentException::class.java) { gate.seekMillis(100.0, 100000) }
        assertEquals(99000, gate.seekMillis(99.0, 100000))
    }
}
