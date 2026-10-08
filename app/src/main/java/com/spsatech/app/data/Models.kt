package com.spsatech.app.data

import kotlinx.serialization.Serializable

@Serializable
data class User(
    val id: Long,
    val login: String,
    val fullName: String,
    val department: String = "",
    /** Подразделения списком; если их несколько, при создании заявки нужно выбрать одно. */
    val departments: List<String> = emptyList(),
    val phone: String = "",
    val position: String = "",
    val role: String,
    val mustChangePassword: Boolean = false,
    /** Пароль совпадает с паролем в «Оценке эффективности» и меняется там. */
    val passwordManagedExternally: Boolean = false,
) {
    val isStaff: Boolean get() = role == "support" || role == "admin"
}

@Serializable
data class LoginRequest(val login: String, val password: String)

@Serializable
data class ChangePasswordRequest(val currentPassword: String, val newPassword: String)

@Serializable
data class LoginResponse(val token: String, val user: User)

@Serializable
data class Person(val id: Long, val fullName: String, val role: String)

@Serializable
data class TicketSummary(
    val id: Long,
    val title: String,
    val status: String,
    val location: String = "",
    val author: Person,
    val messageCount: Int = 0,
    val createdAt: String,
    val updatedAt: String,
)

@Serializable
data class Attachment(
    val id: Long,
    val fileName: String,
    val contentType: String,
    val sizeBytes: Long,
    val createdAt: String,
)

@Serializable
data class Message(
    val id: Long,
    val author: Person,
    val body: String,
    val createdAt: String,
    val attachments: List<Attachment> = emptyList(),
)

@Serializable
data class Ticket(
    val id: Long,
    val title: String,
    val description: String,
    val location: String = "",
    val department: String = "",
    val status: String,
    val author: Person,
    val assignee: Person? = null,
    val createdAt: String,
    val updatedAt: String,
    val attachments: List<Attachment> = emptyList(),
    val messages: List<Message> = emptyList(),
)

@Serializable
data class ApiErrorBody(val error: String, val message: String)

@Serializable
data class DeviceRequest(val token: String, val platform: String = "android")
