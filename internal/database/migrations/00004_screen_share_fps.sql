-- +goose Up
UPDATE user_settings
SET video_quality = 'high'
WHERE video_quality = 'medium';

ALTER TABLE user_settings
DROP CONSTRAINT user_settings_video_quality_check;

ALTER TABLE user_settings
ADD CONSTRAINT user_settings_video_quality_check
CHECK (video_quality IN ('low', 'high'));

-- +goose Down
ALTER TABLE user_settings
DROP CONSTRAINT user_settings_video_quality_check;

ALTER TABLE user_settings
ADD CONSTRAINT user_settings_video_quality_check
CHECK (video_quality IN ('low', 'medium', 'high'));
