package com.spsatech.app.ui.common

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.lifecycle.viewmodel.initializer
import androidx.lifecycle.viewmodel.viewModelFactory
import com.spsatech.app.HelpdeskApp
import com.spsatech.app.data.HelpdeskRepository
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.util.Locale

/** Создаёт ViewModel, передавая ей репозиторий приложения. */
@Composable
inline fun <reified VM : ViewModel> repoViewModel(
    key: String? = null,
    crossinline create: (HelpdeskRepository) -> VM,
): VM {
    val app = LocalContext.current.applicationContext as HelpdeskApp
    return viewModel(key = key, factory = viewModelFactory { initializer { create(app.repository) } })
}

private val dateFormat = DateTimeFormatter.ofPattern("d MMM yyyy, HH:mm", Locale.forLanguageTag("ru"))

fun formatDate(iso: String): String = runCatching {
    OffsetDateTime.parse(iso).atZoneSameInstant(ZoneId.systemDefault()).format(dateFormat)
}.getOrDefault(iso)

data class StatusStyle(val label: String, val color: Color)

fun statusStyle(status: String): StatusStyle = when (status) {
    "new" -> StatusStyle("Новая", Color(0xFF1565C0))
    "in_progress" -> StatusStyle("В работе", Color(0xFFEF6C00))
    "waiting" -> StatusStyle("Ожидает ответа", Color(0xFF6A1B9A))
    "resolved" -> StatusStyle("Решена", Color(0xFF2E7D32))
    "closed" -> StatusStyle("Закрыта", Color(0xFF616161))
    else -> StatusStyle(status, Color(0xFF616161))
}

@Composable
fun StatusChip(status: String, modifier: Modifier = Modifier) {
    val style = statusStyle(status)
    Surface(
        modifier = modifier,
        color = style.color.copy(alpha = 0.12f),
        contentColor = style.color,
        shape = RoundedCornerShape(50),
    ) {
        Text(
            style.label,
            style = MaterialTheme.typography.labelMedium,
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 4.dp),
        )
    }
}

@Composable
fun FullScreenLoading() {
    Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
        CircularProgressIndicator()
    }
}
