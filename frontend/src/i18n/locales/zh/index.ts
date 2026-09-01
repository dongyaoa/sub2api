import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import channelMonitorV2 from './channelMonitorV2'
import batchImage from './batchImage'
import imageStudio from './imageStudio'
import checkin from './checkin'
import mediaStudio from './mediaStudio'
import videoStudio from './videoStudio'
import admin from './admin'
import misc from './misc'
import modelSquare from '../../modelSquare'

export default {
  ...landing,
  ...common,
  ...dashboard,
  ...channelMonitorV2,
  ...batchImage,
  ...imageStudio,
  ...checkin,
  ...mediaStudio,
  ...videoStudio,
  ...modelSquare.zh,
  admin,
  ...misc,
}
