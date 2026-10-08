package com.spsatech.app.push

import android.Manifest
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.content.ContextCompat
import com.spsatech.app.MainActivity
import com.spsatech.app.R

object Notifications {
    const val CHANNEL_TICKETS = "tickets"
    const val EXTRA_TICKET_ID = "ticketId"

    fun createChannels(context: Context) {
        val channel = NotificationChannel(
            CHANNEL_TICKETS, "Заявки", NotificationManager.IMPORTANCE_HIGH,
        ).apply { description = "Ответы техподдержки и изменения статуса заявок" }
        context.getSystemService(NotificationManager::class.java).createNotificationChannel(channel)
    }

    /** Одно уведомление на заявку: новое заменяет предыдущее по той же заявке. */
    fun showTicket(context: Context, ticketId: Long, title: String, body: String) {
        if (ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS)
            != PackageManager.PERMISSION_GRANTED && android.os.Build.VERSION.SDK_INT >= 33
        ) return

        val intent = Intent(context, MainActivity::class.java)
            .putExtra(EXTRA_TICKET_ID, ticketId)
            .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP)
        val pending = PendingIntent.getActivity(
            context, ticketId.toInt(), intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        val notification = NotificationCompat.Builder(context, CHANNEL_TICKETS)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle(title)
            .setContentText(body)
            .setStyle(NotificationCompat.BigTextStyle().bigText(body))
            .setAutoCancel(true)
            .setContentIntent(pending)
            .setPriority(NotificationCompat.PRIORITY_HIGH)
            .build()
        NotificationManagerCompat.from(context).notify(ticketId.toInt(), notification)
    }
}
