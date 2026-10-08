package com.spsatech.app.ui.common

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import coil3.compose.AsyncImage

@Composable
fun PhotoThumb(model: Any?, onClick: () -> Unit, modifier: Modifier = Modifier, size: Dp = 88.dp) {
    AsyncImage(
        model = model,
        contentDescription = "Фото",
        contentScale = ContentScale.Crop,
        modifier = modifier
            .size(size)
            .clip(RoundedCornerShape(8.dp))
            .background(MaterialTheme.colorScheme.surfaceVariant)
            .clickable(onClick = onClick),
    )
}

/** Фото на весь экран; закрывается тапом или кнопкой. */
@Composable
fun PhotoViewer(model: Any, onDismiss: () -> Unit) {
    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Box(
            Modifier
                .fillMaxSize()
                .background(Color.Black)
                .clickable(onClick = onDismiss),
        ) {
            AsyncImage(
                model = model,
                contentDescription = "Фото",
                contentScale = ContentScale.Fit,
                modifier = Modifier.fillMaxSize(),
            )
            IconButton(onClick = onDismiss, modifier = Modifier.align(Alignment.TopEnd).padding(16.dp)) {
                Icon(Icons.Default.Close, contentDescription = "Закрыть", tint = Color.White)
            }
        }
    }
}
