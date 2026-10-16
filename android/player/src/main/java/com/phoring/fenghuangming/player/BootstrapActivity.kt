package com.phoring.fenghuangming.player

import android.app.Activity
import android.content.Intent
import android.os.Build
import android.os.Bundle

class BootstrapActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        if (Build.VERSION.SDK_INT >= 33 && checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS) != android.content.pm.PackageManager.PERMISSION_GRANTED) {
            requestPermissions(arrayOf(android.Manifest.permission.POST_NOTIFICATIONS), 1)
        } else startPlayer()
    }
    private fun startPlayer() {
        startForegroundService(Intent(this, PlaybackService::class.java))
        finish()
    }
    override fun onRequestPermissionsResult(requestCode: Int, permissions: Array<out String>, grantResults: IntArray) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        startPlayer()
    }
}
