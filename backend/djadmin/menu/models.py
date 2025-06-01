from django.db import models
from role.models import SysRole
# Create your models here.


class SysMenu(models.Model):
    id = models.AutoField(primary_key=True)
    name = models.CharField(max_length=50, unique=True, verbose_name="菜单名称")
    icon = models.CharField(max_length=100, null=True, verbose_name="菜单图标")
    parent_id = models.IntegerField(null=True, verbose_name="父菜单ID")
    order_num = models.IntegerField(null=True, verbose_name="显示顺序")
    path = models.CharField(max_length=200, null=True, verbose_name="路由地址")
    component = models.CharField(max_length=255, null=True, verbose_name="组件路径")
    menu_type = models.CharField(max_length=1, null=True, verbose_name="菜单类型（M目录 C菜单 F按钮）")
    perms = models.CharField(max_length=100, null=True, verbose_name="权限标识")
    create_time = models.DateField(null=True,blank=True, verbose_name="创建时间", )
    update_time = models.DateField(null=True,blank=True, verbose_name="更新时间")
    remark = models.CharField(max_length=500,blank=True, null=True, verbose_name="备注")
    class Meta:
        db_table = "sys_menu"

    # children = list()
    def __lt__(self, other):
        return self.order_num < other.order_num




# 系统角色菜单关联类
class SysRoleMenu(models.Model):
    id = models.AutoField(primary_key=True)
    role = models.ForeignKey(SysRole, on_delete=models.CASCADE)
    menu = models.ForeignKey(SysMenu, on_delete=models.CASCADE)
    class Meta:
        db_table = "sys_role_menu"
        unique_together = ('menu', 'role')
