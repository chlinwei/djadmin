<template>
    <svg :style="{ width,height,verticalAlign }">
        <use :href="prefix + name" :fill="color"></use>
    </svg>
</template>


<script setup>
    defineProps({
        prefix: {
            type: String,
            default: "#icon-"
        },
        name: String,
        color: {
            type: String,
            default: "gray"
        },
        width: {
            type: String,
            default: "16px"          
        },
        height: {
            type: String,
            default: "16px"          
        },
        verticalAlign: {
            type: String,
            default: "middle" 
        }
        
    })
</script>