package com.spsatech.app.push

import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage
import com.spsatech.app.HelpdeskApp

/**
 * Сервер шлёт data-сообщения (ticketId, title, body): так уведомление
 * всегда рисует приложение — и в фоне, и когда оно открыто.
 */
class PushService : FirebaseMessagingService() {
    override fun onNewToken(token: String) {
        (application as HelpdeskApp).pushRegistrar.register(token)
    }

    override fun onMessageReceived(message: RemoteMessage) {
        val data = message.data
        val ticketId = data["ticketId"]?.toLongOrNull() ?: return
        val app = application as HelpdeskApp
        app.ticketUpdates.tryEmit(ticketId)
        Notifications.showTicket(this, ticketId, data["title"].orEmpty(), data["body"].orEmpty())
    }
}
