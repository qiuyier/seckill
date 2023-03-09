if tonumber(redis.call('get', KEYS[1])) > 0
then
    if tonumber(redis.call('incr',KEYS[2])) > 1
    then
        return 2
    else
        redis.call('decr',KEYS[1])
        return 1
    end
else
    return 0
end