from django.db import migrations


# WebSSH 终端输出可能包含 emoji 等 4 字节 UTF-8 字符（如 🔰），
# 若列字符集为 utf8mb3 会触发 MySQL "Incorrect string value" 写入失败。
# 将存储会话内容的文本列转换为 utf8mb4，使其可容纳全部 Unicode 字符。
TARGET_TABLE = 'assets_webssh_session_log'
TARGET_COLUMNS = ('input_content', 'output_content', 'error_message')

FORWARD_SQL = [
    f"ALTER TABLE {TARGET_TABLE} MODIFY {column} LONGTEXT "
    f"CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL"
    for column in TARGET_COLUMNS
]


class Migration(migrations.Migration):

    dependencies = [
        ('assets', '0032_websshtempcredential'),
    ]

    operations = [
        migrations.RunSQL(
            sql=FORWARD_SQL,
            # 反向操作为无操作：utf8mb4 向下兼容，无需回退到 utf8mb3。
            reverse_sql=migrations.RunSQL.noop,
        ),
    ]
